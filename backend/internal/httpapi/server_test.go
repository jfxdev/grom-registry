package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	auditdomain "github.com/jfxdev/grom/backend/internal/audit/domain"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	platformbackup "github.com/jfxdev/grom/backend/internal/platform/backup"
	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
)

type serverTestProjectRepository struct {
	projectdomain.Repository
	project *projectdomain.Project
}

type serverTestAuditStore struct {
	mu     sync.Mutex
	events []*auditdomain.Event
	err    error
}

func (store *serverTestAuditStore) Record(_ context.Context, event *auditdomain.Event) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.events = append(store.events, event)
	return store.err
}

func (store *serverTestAuditStore) RecordOnce(ctx context.Context, event *auditdomain.Event) error {
	return store.Record(ctx, event)
}

type serverTestBackupAgent struct {
	mu               sync.Mutex
	available        bool
	summaries        []platformbackup.Summary
	listErr          error
	createErr        error
	deleteErr        error
	downloadErr      error
	downloadResponse *http.Response
	createStarted    chan struct{}
	createRelease    chan struct{}
}

func (agent *serverTestBackupAgent) List(context.Context) ([]platformbackup.Summary, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return append([]platformbackup.Summary(nil), agent.summaries...), agent.listErr
}

func (agent *serverTestBackupAgent) Create(context.Context, platformbackup.AgentCreateRequest) (platformbackup.Summary, error) {
	if agent.createStarted != nil {
		select {
		case agent.createStarted <- struct{}{}:
		default:
		}
	}
	if agent.createRelease != nil {
		<-agent.createRelease
	}
	return platformbackup.Summary{BackupID: foundation.NewID().String(), GromVersion: "test"}, agent.createErr
}

func (agent *serverTestBackupAgent) Download(context.Context, string) (*http.Response, error) {
	if agent.downloadErr != nil {
		return nil, agent.downloadErr
	}
	return agent.downloadResponse, nil
}

func (agent *serverTestBackupAgent) Delete(context.Context, string) error {
	return agent.deleteErr
}

func (agent *serverTestBackupAgent) Available(context.Context) bool {
	return agent.available
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

func TestBackupHandlersRequireInstallationAdministrator(t *testing.T) {
	server := &Server{}
	for _, test := range []struct {
		name    string
		method  string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "list", method: http.MethodGet, handler: server.listBackups},
		{name: "create", method: http.MethodPost, handler: server.createBackup},
		{name: "delete", method: http.MethodDelete, handler: server.deleteBackup},
		{name: "download", method: http.MethodGet, handler: server.downloadBackup},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := backupAdminRequest(test.method, "http://backend/api/v1/backups", false)
			response := httptest.NewRecorder()
			test.handler(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
			}
		})
	}
}

func TestListBackupsReportsAvailabilityPaginationAndInvalidCursors(t *testing.T) {
	unavailable := httptest.NewRecorder()
	(&Server{}).listBackups(
		unavailable,
		backupAdminRequest(http.MethodGet, "http://backend/api/v1/backups", true),
	)
	if unavailable.Code != http.StatusOK ||
		!strings.Contains(unavailable.Body.String(), `"available":false`) ||
		!strings.Contains(unavailable.Body.String(), `"pageSize":5`) {
		t.Fatalf("unexpected unavailable response %d: %s", unavailable.Code, unavailable.Body.String())
	}

	agent := &serverTestBackupAgent{
		available: true,
		summaries: []platformbackup.Summary{{
			BackupID: foundation.NewID().String(), CreatedAt: time.Now().UTC().Format(time.RFC3339),
			GromVersion: "test", TotalBytes: 42,
		}},
	}
	server := backupTestServer(agent, nil)
	listed := httptest.NewRecorder()
	server.listBackups(listed, backupAdminRequest(http.MethodGet, "http://backend/api/v1/backups", true))
	if listed.Code != http.StatusOK ||
		!strings.Contains(listed.Body.String(), `"available":true`) ||
		!strings.Contains(listed.Body.String(), `"totalBackups":1`) {
		t.Fatalf("unexpected list response %d: %s", listed.Code, listed.Body.String())
	}

	for _, rawURL := range []string{
		"http://backend/api/v1/backups?cursor=invalid",
		"http://backend/api/v1/backups?cursor=" + strings.Repeat("x", 513),
	} {
		response := httptest.NewRecorder()
		server.listBackups(response, backupAdminRequest(http.MethodGet, rawURL, true))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_cursor") {
			t.Fatalf("expected invalid cursor response, got %d: %s", response.Code, response.Body.String())
		}
	}
}

func TestCreateBackupMapsAvailabilityAndConcurrency(t *testing.T) {
	unavailable := httptest.NewRecorder()
	(&Server{}).createBackup(
		unavailable,
		backupAdminRequest(http.MethodPost, "http://backend/api/v1/backups", true),
	)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing manager status %d, got %d", http.StatusServiceUnavailable, unavailable.Code)
	}

	offline := backupTestServer(&serverTestBackupAgent{}, nil)
	offlineResponse := httptest.NewRecorder()
	offline.createBackup(
		offlineResponse,
		backupAdminRequest(http.MethodPost, "http://backend/api/v1/backups", true),
	)
	if offlineResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected offline agent status %d, got %d", http.StatusServiceUnavailable, offlineResponse.Code)
	}

	release := make(chan struct{})
	agent := &serverTestBackupAgent{
		available: true, createStarted: make(chan struct{}, 1), createRelease: release,
	}
	server := backupTestServer(agent, nil)
	accepted := httptest.NewRecorder()
	server.createBackup(accepted, backupAdminRequest(http.MethodPost, "http://backend/api/v1/backups", true))
	if accepted.Code != http.StatusAccepted || !strings.Contains(accepted.Body.String(), `"status":"starting"`) {
		t.Fatalf("unexpected accepted response %d: %s", accepted.Code, accepted.Body.String())
	}

	conflict := httptest.NewRecorder()
	server.createBackup(conflict, backupAdminRequest(http.MethodPost, "http://backend/api/v1/backups", true))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "backup_active") {
		t.Fatalf("unexpected conflict response %d: %s", conflict.Code, conflict.Body.String())
	}
	close(release)
}

func TestDeleteBackupMapsErrorsAndRecordsAudit(t *testing.T) {
	backupID := foundation.NewID().String()
	missingManager := httptest.NewRecorder()
	(&Server{}).deleteBackup(
		missingManager,
		backupRequestWithID(http.MethodDelete, backupID),
	)
	if missingManager.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing manager status %d, got %d", http.StatusServiceUnavailable, missingManager.Code)
	}

	invalid := httptest.NewRecorder()
	backupTestServer(&serverTestBackupAgent{available: true}, nil).deleteBackup(
		invalid,
		backupRequestWithID(http.MethodDelete, "not-a-uuid"),
	)
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("expected invalid ID status %d, got %d", http.StatusNotFound, invalid.Code)
	}

	auditStore := &serverTestAuditStore{}
	server := backupTestServer(&serverTestBackupAgent{available: true}, auditStore)
	deleted := httptest.NewRecorder()
	server.deleteBackup(deleted, backupRequestWithID(http.MethodDelete, backupID))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("expected delete status %d, got %d: %s", http.StatusNoContent, deleted.Code, deleted.Body.String())
	}
	if len(auditStore.events) != 2 ||
		auditStore.events[0].Action != constants.AuditBackupDeleteRequested ||
		auditStore.events[1].Action != constants.AuditBackupDeleted {
		t.Fatalf("unexpected delete audit events: %#v", auditStore.events)
	}

	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "not found", err: os.ErrNotExist, status: http.StatusNotFound, code: "backup_not_found"},
		{name: "agent failure", err: errors.New("agent failed"), status: http.StatusServiceUnavailable, code: "backup_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			backupTestServer(&serverTestBackupAgent{available: true, deleteErr: test.err}, nil).
				deleteBackup(response, backupRequestWithID(http.MethodDelete, backupID))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("expected %d/%s, got %d: %s", test.status, test.code, response.Code, response.Body.String())
			}
		})
	}

	release := make(chan struct{})
	activeServer := backupTestServer(&serverTestBackupAgent{
		available: true, createStarted: make(chan struct{}, 1), createRelease: release,
	}, nil)
	if _, err := activeServer.backups.Start(); err != nil {
		t.Fatal(err)
	}
	conflict := httptest.NewRecorder()
	activeServer.deleteBackup(conflict, backupRequestWithID(http.MethodDelete, backupID))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "backup_active") {
		t.Fatalf("unexpected active delete response %d: %s", conflict.Code, conflict.Body.String())
	}
	close(release)
}

func TestDownloadBackupStreamsOnlySuccessfulAgentResponse(t *testing.T) {
	backupID := foundation.NewID().String()
	for _, test := range []struct {
		name   string
		server *Server
		status int
	}{
		{name: "missing manager", server: &Server{}, status: http.StatusNotFound},
		{
			name: "agent failure",
			server: backupTestServer(&serverTestBackupAgent{
				available: true, downloadErr: errors.New("missing"),
			}, nil),
			status: http.StatusNotFound,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			test.server.downloadBackup(response, backupRequestWithID(http.MethodGet, backupID))
			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d", test.status, response.Code)
			}
		})
	}

	server := backupTestServer(&serverTestBackupAgent{
		available: true,
		downloadResponse: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Disposition": []string{`attachment; filename="backup.tar"`},
			},
			Body: io.NopCloser(strings.NewReader("portable bundle")),
		},
	}, nil)
	response := httptest.NewRecorder()
	server.downloadBackup(response, backupRequestWithID(http.MethodGet, backupID))
	if response.Code != http.StatusOK ||
		response.Header().Get("Content-Type") != "application/x-tar" ||
		response.Header().Get("Content-Disposition") != `attachment; filename="backup.tar"` ||
		response.Body.String() != "portable bundle" {
		t.Fatalf("unexpected download response %d %#v: %q", response.Code, response.Header(), response.Body.String())
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

func backupTestServer(agent platformbackup.Agent, auditStore *serverTestAuditStore) *Server {
	if auditStore == nil {
		auditStore = &serverTestAuditStore{}
	}
	return &Server{
		backups: platformbackup.NewManager(
			agent,
			maintenance.New(),
			func(context.Context) error { return nil },
			"test-version",
			"development",
			nil,
		),
		audit:  auditapp.New(auditStore),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestRequiresQuiescenceTrackingNormalizesSecuritySensitivePaths(t *testing.T) {
	for _, test := range []struct {
		method string
		target string
		want   bool
	}{
		{method: http.MethodGet, target: "http://grom/ignored/%2e%2e/auth/token", want: true},
		{method: http.MethodGet, target: "http://grom/ignored/%2e%2e/v2/manifests/latest", want: true},
		{method: http.MethodGet, target: "http://grom//v2//blobs/digest", want: true},
		{method: http.MethodPost, target: "http://grom/api//v1/./backups", want: false},
	} {
		request := httptest.NewRequest(test.method, test.target, nil)
		if actual := requiresQuiescenceTracking(request); actual != test.want {
			t.Fatalf("%s %s: expected %v, got %v", test.method, test.target, test.want, actual)
		}
	}
}

func backupAdminRequest(method, target string, systemAdmin bool) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	return request.WithContext(context.WithValue(request.Context(), currentUserKey{}, &identitydomain.User{
		ID: foundation.NewID(), SystemAdmin: systemAdmin,
	}))
}

func backupRequestWithID(method, backupID string) *http.Request {
	request := backupAdminRequest(method, "http://backend/api/v1/backups/"+backupID, true)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("backupId", backupID)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
