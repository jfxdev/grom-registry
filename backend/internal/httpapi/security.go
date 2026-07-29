package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxAuthenticationLimiterEntries = 10_000

type SecurityOptions struct {
	TrustedProxies    []netip.Prefix
	AuthFailureLimit  int
	AuthFailureWindow time.Duration
	AuthBlockDuration time.Duration
}

type authenticationFailure struct {
	failures      int
	windowStarted time.Time
	blockedUntil  time.Time
	lastSeen      time.Time
}

type authenticationFailureLimiter struct {
	mu            sync.Mutex
	entries       map[string]authenticationFailure
	failureLimit  int
	failureWindow time.Duration
	blockDuration time.Duration
	maxEntries    int
}

func newAuthenticationFailureLimiter(options SecurityOptions) *authenticationFailureLimiter {
	return &authenticationFailureLimiter{
		entries:       make(map[string]authenticationFailure),
		failureLimit:  options.AuthFailureLimit,
		failureWindow: options.AuthFailureWindow,
		blockDuration: options.AuthBlockDuration,
		maxEntries:    maxAuthenticationLimiterEntries,
	}
}

func (l *authenticationFailureLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.entries[key]
	if !exists {
		return true, 0
	}
	if now.Before(entry.blockedUntil) {
		return false, entry.blockedUntil.Sub(now)
	}
	if now.Sub(entry.windowStarted) >= l.failureWindow {
		delete(l.entries, key)
	}
	return true, 0
}

func (l *authenticationFailureLimiter) failure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.entries[key]
	if !exists || now.Sub(entry.windowStarted) >= l.failureWindow {
		entry = authenticationFailure{windowStarted: now}
	}
	entry.failures++
	entry.lastSeen = now
	if entry.failures >= l.failureLimit {
		entry.blockedUntil = now.Add(l.blockDuration)
	}
	l.entries[key] = entry
	l.enforceBound(now)
}

func (l *authenticationFailureLimiter) success(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func (l *authenticationFailureLimiter) enforceBound(now time.Time) {
	if len(l.entries) <= l.maxEntries {
		return
	}
	for key, entry := range l.entries {
		if !now.Before(entry.blockedUntil) && now.Sub(entry.lastSeen) >= l.failureWindow {
			delete(l.entries, key)
		}
	}
	for len(l.entries) > l.maxEntries {
		var oldestKey string
		var oldestSeen time.Time
		for key, entry := range l.entries {
			if oldestKey == "" || entry.lastSeen.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = entry.lastSeen
			}
		}
		delete(l.entries, oldestKey)
	}
}

func (s *Server) trustedRealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		immediatePeer, ok := remoteAddress(r.RemoteAddr)
		if !ok || !s.isTrustedProxy(immediatePeer) {
			next.ServeHTTP(w, r)
			return
		}

		client, found := s.forwardedClientAddress(r, immediatePeer)
		if !found {
			next.ServeHTTP(w, r)
			return
		}
		cloned := r.Clone(r.Context())
		cloned.RemoteAddr = client.String()
		next.ServeHTTP(w, cloned)
	})
}

func (s *Server) forwardedClientAddress(r *http.Request, immediatePeer netip.Addr) (netip.Addr, bool) {
	forwardedFor := r.Header.Values("X-Forwarded-For")
	if len(forwardedFor) == 0 {
		if realIP, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
			return realIP.Unmap(), true
		}
		return netip.Addr{}, false
	}

	chain := make([]string, 0, len(forwardedFor))
	for _, header := range forwardedFor {
		chain = append(chain, strings.Split(header, ",")...)
	}
	current := immediatePeer
	for index := len(chain) - 1; index >= 0; index-- {
		candidate, err := netip.ParseAddr(strings.TrimSpace(chain[index]))
		if err != nil {
			return netip.Addr{}, false
		}
		candidate = candidate.Unmap()
		if !s.isTrustedProxy(current) {
			break
		}
		current = candidate
	}
	return current, current != immediatePeer
}

func (s *Server) isTrustedProxy(address netip.Addr) bool {
	for _, prefix := range s.trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func remoteAddress(value string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	address, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func authenticationLimiterKey(r *http.Request) string {
	if address, ok := remoteAddress(r.RemoteAddr); ok {
		return address.String()
	}
	return "unknown"
}

func writeRateLimitError(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	seconds := max(int((retryAfter+time.Second-1)/time.Second), 1)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, r, http.StatusTooManyRequests, "rate_limited", "Too many failed authentication attempts")
}
