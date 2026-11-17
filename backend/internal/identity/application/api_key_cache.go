package application

import (
	"crypto/sha256"
	"crypto/subtle"
	"sync"
	"time"
)

// apiKeyVerificationCache remembers recent successful Argon2id verifications
// of access-key secrets. Stateless clients such as MCP agents authenticate
// every request, and each Argon2id check costs 64 MiB and noticeable CPU.
//
// An entry is bound to the stored hash and holds only a SHA-256 digest of the
// high-entropy secret, never the secret itself. Callers still load the token
// and its owner on every request, so revocation, expiry and disabled owners
// take effect immediately; a hit skips only the hash computation.
type apiKeyVerificationCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
	entries    map[string]apiKeyVerification
}

type apiKeyVerification struct {
	storedHash   string
	secretDigest [sha256.Size]byte
	expiresAt    time.Time
}

func newAPIKeyVerificationCache(ttl time.Duration, maxEntries int) *apiKeyVerificationCache {
	return &apiKeyVerificationCache{
		ttl: ttl, maxEntries: maxEntries, now: time.Now,
		entries: make(map[string]apiKeyVerification),
	}
}

// verify reports whether secret matches storedHash, consulting the cache before
// falling back to the expensive verifier and caching only successes.
func (c *apiKeyVerificationCache) verify(tokenID, storedHash, secret string, verifier func(string, string) bool) bool {
	digest := sha256.Sum256([]byte(secret))
	now := c.now()
	c.mu.Lock()
	entry, ok := c.entries[tokenID]
	c.mu.Unlock()
	if ok && now.Before(entry.expiresAt) && entry.storedHash == storedHash &&
		subtle.ConstantTimeCompare(entry.secretDigest[:], digest[:]) == 1 {
		return true
	}
	if !verifier(storedHash, secret) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[tokenID]; !exists && len(c.entries) >= c.maxEntries {
		c.evictLocked(now)
	}
	c.entries[tokenID] = apiKeyVerification{storedHash: storedHash, secretDigest: digest, expiresAt: now.Add(c.ttl)}
	return true
}

func (c *apiKeyVerificationCache) evictLocked(now time.Time) {
	for id, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
	for id := range c.entries {
		if len(c.entries) < c.maxEntries {
			return
		}
		delete(c.entries, id)
	}
}
