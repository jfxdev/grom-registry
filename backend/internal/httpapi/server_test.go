package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
)

type serverTestProjectRepository struct {
	projectdomain.Repository
	project *projectdomain.Project
}

func (r *serverTestProjectRepository) FindProjectBySlug(
	context.Context,
	string,
) (*projectdomain.Project, error) {
	return r.project, nil
}

func TestOriginGuardAllowsPublicAndForwardedDevelopmentOrigins(t *testing.T) {
	publicURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{publicURL: publicURL}
	handler := server.originGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name   string
		host   string
		origin string
		status int
	}{
		{name: "configured public URL", host: "localhost:8080", origin: "http://localhost:8080", status: http.StatusNoContent},
		{name: "Vite forwarded host", host: "localhost:5173", origin: "http://localhost:5173", status: http.StatusNoContent},
		{name: "foreign origin", host: "localhost:8080", origin: "https://example.com", status: http.StatusForbidden},
		{name: "invalid origin", host: "localhost:8080", origin: "null", status: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://backend/api/v1/me/password", nil)
			request.Host = test.host
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d", test.status, response.Code)
			}
		})
	}
}

func TestOriginGuardRequiresOriginForSessionCookieMutations(t *testing.T) {
	publicURL, err := url.Parse("https://registry.example.com")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{publicURL: publicURL}
	handler := server.originGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name       string
		origin     string
		withCookie bool
		status     int
	}{
		{name: "session cookie without origin", withCookie: true, status: http.StatusForbidden},
		{name: "unauthenticated API client without origin", status: http.StatusNoContent},
		{name: "canonical HTTPS origin", origin: "https://registry.example.com", withCookie: true, status: http.StatusNoContent},
		{name: "same host with insecure scheme", origin: "http://registry.example.com", withCookie: true, status: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://registry.example.com/api/v1/projects", nil)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.withCookie {
				request.AddCookie(&http.Cookie{Name: constants.SessionCookieName, Value: "session"})
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d", test.status, response.Code)
			}
		})
	}
}

func TestSessionCookieUsesBrowserSecurityAttributes(t *testing.T) {
	server := &Server{secureCookies: true}
	response := httptest.NewRecorder()

	server.setSessionCookie(response, "session-secret")

	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected HttpOnly, Secure, SameSite=Lax cookie, got %+v", cookie)
	}
	if cookie.Path != "/" {
		t.Fatalf("expected root cookie path, got %q", cookie.Path)
	}
}

func TestDeploymentPostureExposesOnlyOperatorWarningState(t *testing.T) {
	server := &Server{deploymentProfile: "permissive", insecureHTTP: true}
	request := httptest.NewRequest(http.MethodGet, "http://grom/api/v1/deployment", nil)
	response := httptest.NewRecorder()

	server.getDeployment(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected application/json, got %q", contentType)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"insecureHttp":true,"profile":"permissive"}` {
		t.Fatalf("unexpected deployment posture response: %s", body)
	}
}

func TestTrustedRealIPIgnoresHeadersFromUntrustedPeers(t *testing.T) {
	trustedProxy := netip.MustParsePrefix("10.0.0.0/8")
	server := &Server{trustedProxies: []netip.Prefix{trustedProxy}}
	handler := server.trustedRealIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.RemoteAddr)
	}))

	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		expected   string
	}{
		{
			name:       "untrusted peer cannot spoof",
			remoteAddr: "198.51.100.20:4321",
			forwarded:  "203.0.113.10",
			expected:   "198.51.100.20:4321",
		},
		{
			name:       "trusted proxy supplies client",
			remoteAddr: "10.0.0.5:4321",
			forwarded:  "203.0.113.10",
			expected:   "203.0.113.10",
		},
		{
			name:       "rightmost trusted chain is skipped",
			remoteAddr: "10.0.0.5:4321",
			forwarded:  "203.0.113.10, 10.0.0.6",
			expected:   "203.0.113.10",
		},
		{
			name:       "malformed trusted header is ignored",
			remoteAddr: "10.0.0.5:4321",
			forwarded:  "not-an-address",
			expected:   "10.0.0.5:4321",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://grom/healthz", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if actual := strings.TrimSpace(response.Body.String()); actual != test.expected {
				t.Fatalf("expected remote address %q, got %q", test.expected, actual)
			}
		})
	}
}

func TestAuthenticationFailureLimiterBlocksOnlyFailingClient(t *testing.T) {
	options := SecurityOptions{
		AuthFailureLimit: 2, AuthFailureWindow: time.Minute, AuthBlockDuration: 10 * time.Minute,
	}
	limiter := newAuthenticationFailureLimiter(options)
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)

	limiter.failure("198.51.100.10", now)
	limiter.failure("198.51.100.10", now.Add(time.Second))

	if allowed, _ := limiter.allow("198.51.100.10", now.Add(2*time.Second)); allowed {
		t.Fatal("expected failing client to be blocked")
	}
	if allowed, _ := limiter.allow("198.51.100.11", now.Add(2*time.Second)); !allowed {
		t.Fatal("expected unrelated client to remain allowed")
	}

	limiter.success("198.51.100.10")
	if allowed, _ := limiter.allow("198.51.100.10", now.Add(3*time.Second)); !allowed {
		t.Fatal("expected successful authentication to clear limiter state")
	}
}

func TestAuthenticationFailureLimiterBoundsMemory(t *testing.T) {
	limiter := newAuthenticationFailureLimiter(SecurityOptions{
		AuthFailureLimit: 5, AuthFailureWindow: time.Minute, AuthBlockDuration: 10 * time.Minute,
	})
	limiter.maxEntries = 2
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)

	limiter.failure("198.51.100.10", now)
	limiter.failure("198.51.100.11", now.Add(time.Second))
	limiter.failure("198.51.100.12", now.Add(2*time.Second))

	if len(limiter.entries) != 2 {
		t.Fatalf("expected limiter to retain at most two entries, got %d", len(limiter.entries))
	}
}

func TestAccessLogDoesNotRecordAuthorizationHeader(t *testing.T) {
	var output bytes.Buffer
	server := &Server{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	handler := server.accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	request := httptest.NewRequest(http.MethodGet, "http://grom/auth/token?service=registry", nil)
	request.Header.Set("Authorization", "Basic secret-credential")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if strings.Contains(output.String(), "secret-credential") ||
		strings.Contains(strings.ToLower(output.String()), "authorization") {
		t.Fatalf("access log exposed authentication material: %s", output.String())
	}
}

func TestCreateProjectRequiresInstallationAdministrator(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPost, "http://backend/api/v1/projects", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey{}, &identitydomain.User{
		SystemAdmin: false,
	}))
	response := httptest.NewRecorder()

	server.createProject(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
}

func TestDeleteProjectRequiresInstallationAdministrator(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodDelete, "http://backend/api/v1/projects/payments", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey{}, &identitydomain.User{
		SystemAdmin: false,
	}))
	response := httptest.NewRecorder()

	server.deleteProject(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
}

func TestListServiceAccountsRequiresInstallationAdministrator(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "http://backend/api/v1/service-accounts", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey{}, &identitydomain.User{
		SystemAdmin: false,
	}))
	response := httptest.NewRecorder()

	server.listServiceAccounts(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
}

func TestLifecycleMutationsRejectMissingRequiredIdentifiers(t *testing.T) {
	project := &projectdomain.Project{ID: foundation.NewID(), Slug: "payments"}
	server := &Server{
		projects: projectapp.New(&serverTestProjectRepository{project: project}),
	}
	testCases := []struct {
		name      string
		handler   func(http.ResponseWriter, *http.Request)
		errorCode string
	}{
		{name: "inventory reconciliation", handler: server.reconcileRepositoryInventory, errorCode: "invalid_repository"},
		{name: "lifecycle preview", handler: server.createLifecyclePreview, errorCode: "invalid_repository"},
		{name: "lifecycle run", handler: server.createLifecycleRun, errorCode: "invalid_preview"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://backend/api/v1/projects/payments", strings.NewReader("{}"))
			request = request.WithContext(context.WithValue(request.Context(), currentUserKey{}, &identitydomain.User{
				ID: foundation.NewID(), SystemAdmin: true,
			}))
			response := httptest.NewRecorder()

			testCase.handler(response, request)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), testCase.errorCode) {
				t.Fatalf("expected validation error %q, got status %d: %s", testCase.errorCode, response.Code, response.Body.String())
			}
		})
	}
}
