package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/mcp"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
)

func (s *Server) newMCPHandler(version string) http.Handler {
	deps := mcp.Deps{
		Projects:      s.projects,
		Repositories:  s.repositories,
		PolicyPresets: registryapp.PolicyPresets,
		Logger:        s.logger,
	}
	// Typed nil service pointers would become non-nil interfaces; leave the
	// optional readers unset so the tools report them as unavailable.
	if s.inventory != nil {
		deps.Inventory = s.inventory
	}
	if s.lifecycle != nil {
		deps.Lifecycle = s.lifecycle
	}
	if s.artifactDeletions != nil {
		deps.Deletions = s.artifactDeletions
	}
	return mcp.NewHandler(deps, version)
}

// requireAPITokenBearer authenticates the MCP endpoint with a registry access
// key presented as a bearer token. It shares the registry token endpoint's
// failure limiter so the endpoint never adds attempts for the same credential
// type, and it never reads the web session cookie.
func (s *Server) requireAPITokenBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		limiterKey := authenticationLimiterKey(r)
		if allowed, retryAfter := s.registryLimiter.allow(limiterKey, time.Now()); !allowed {
			writeRateLimitError(w, r, retryAfter)
			return
		}
		rawToken, ok := bearerToken(r)
		if !ok {
			s.rejectMCPAuthentication(w, r, limiterKey, "missing_bearer_token", "API access key required")
			return
		}
		principal, err := s.identity.AuthenticateAPIToken(r.Context(), rawToken)
		if err != nil {
			s.rejectMCPAuthentication(w, r, limiterKey, "invalid_credentials", "Invalid API access key")
			return
		}
		s.registryLimiter.success(limiterKey)
		next.ServeHTTP(w, r.WithContext(mcp.WithPrincipal(r.Context(), principal)))
	})
}

func (s *Server) rejectMCPAuthentication(w http.ResponseWriter, r *http.Request, limiterKey, reason, message string) {
	s.registryLimiter.failure(limiterKey, time.Now())
	_ = s.recordAudit(r, anonymousPrincipal(), constants.AuditRegistryAuthFailed, constants.AuditResourceAuthentication, "", map[string]any{"reason": reason, "channel": "mcp"})
	w.Header().Set("WWW-Authenticate", `Bearer realm="`+constants.MCPAuthRealm+`"`)
	writeError(w, r, http.StatusUnauthorized, "unauthenticated", message)
}

func bearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || len(values[0]) > constants.MCPMaxBearerLength {
		return "", false
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}
