package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	identitystore "github.com/jfxdev/grom/backend/internal/identity/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectstore "github.com/jfxdev/grom/backend/internal/projects/infrastructure/persistence/bun"
)

func newFlowTestServer(t *testing.T) (*Server, *identityapp.Service, *identitydomain.User) {
	t.Helper()
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:httpapi-flow-"+foundation.NewID().String()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	identity := identityapp.New(identitystore.New(db), time.Hour)
	if err := identity.BootstrapAdmin(ctx, "admin@example.com", "admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := identity.Login(ctx, "admin@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	projects := projectapp.New(projectstore.New(db))
	server := &Server{
		identity: identity, audit: auditapp.New(&serverTestAuditStore{}), projects: projects,
		publicURL:       &url.URL{Scheme: "https", Host: "grom.example"},
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		loginLimiter:    newAuthenticationFailureLimiter(SecurityOptions{AuthFailureLimit: 10, AuthFailureWindow: time.Minute, AuthBlockDuration: time.Minute}),
		registryLimiter: newAuthenticationFailureLimiter(SecurityOptions{AuthFailureLimit: 10, AuthFailureWindow: time.Minute, AuthBlockDuration: time.Minute}),
	}
	return server, identity, admin
}

func TestCurrentUserReturnsTheContextUser(t *testing.T) {
	server, _, admin := newFlowTestServer(t)
	response := httptest.NewRecorder()

	server.currentUser(response, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/me", nil), admin))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var decoded identitydomain.User
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != admin.ID {
		t.Fatalf("expected the current user, got %#v", decoded)
	}
}

func TestGetInstallationStatusWithoutOptionalDependencies(t *testing.T) {
	server, _, admin := newFlowTestServer(t)
	response := httptest.NewRecorder()

	server.getInstallationStatus(response, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/settings/status", nil), admin))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"distribution":"unavailable"`) {
		t.Fatalf("expected distribution to report unavailable, got %s", response.Body.String())
	}
}

func TestGetInstallationStatusRequiresAdministrator(t *testing.T) {
	server, _, _ := newFlowTestServer(t)
	response := httptest.NewRecorder()
	nonAdmin := &identitydomain.User{ID: foundation.NewID()}

	server.getInstallationStatus(response, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/settings/status", nil), nonAdmin))

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestRunGarbageCollectionWithoutMaintenanceAgentIsUnavailable(t *testing.T) {
	server, _, admin := newFlowTestServer(t)
	response := httptest.NewRecorder()

	server.runGarbageCollection(response, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/garbage-collections", nil), admin))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no registry maintenance agent is configured, got %d", response.Code)
	}
}

func TestChangeCurrentUserPassword(t *testing.T) {
	server, identity, admin := newFlowTestServer(t)

	wrongCurrent := httptest.NewRecorder()
	server.changeCurrentUserPassword(wrongCurrent, withUser(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/me/password", strings.NewReader(`{"currentPassword":"wrong","newPassword":"new-strong-password"}`)), admin))
	if wrongCurrent.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a wrong current password, got %d: %s", wrongCurrent.Code, wrongCurrent.Body.String())
	}

	tooShort := httptest.NewRecorder()
	server.changeCurrentUserPassword(tooShort, withUser(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/me/password", strings.NewReader(`{"currentPassword":"secret-password","newPassword":"short"}`)), admin))
	if tooShort.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a too-short new password, got %d", tooShort.Code)
	}

	success := httptest.NewRecorder()
	server.changeCurrentUserPassword(success, withUser(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/me/password", strings.NewReader(`{"currentPassword":"secret-password","newPassword":"new-strong-password"}`)), admin))
	if success.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", success.Code, success.Body.String())
	}
	if _, _, err := identity.Login(context.Background(), "admin@example.com", "new-strong-password"); err != nil {
		t.Fatalf("expected login with the new password to succeed, got %v", err)
	}
}

func TestDeleteSessionClearsTheCookie(t *testing.T) {
	server, identity, _ := newFlowTestServer(t)
	raw, _, err := identity.Login(context.Background(), "admin@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: constants.SessionCookieName, Value: raw})
	response := httptest.NewRecorder()

	server.deleteSession(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	if _, err := identity.AuthenticateSession(context.Background(), raw); err == nil {
		t.Fatal("expected the session to be revoked after deleteSession")
	}
	setCookie := response.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, constants.SessionCookieName) || !strings.Contains(setCookie, "Max-Age=0") {
		t.Fatalf("expected the session cookie to be cleared, got %q", setCookie)
	}

	// Deleting a session without a cookie is a harmless no-op.
	noCookieResponse := httptest.NewRecorder()
	server.deleteSession(noCookieResponse, httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/session", nil))
	if noCookieResponse.Code != http.StatusNoContent {
		t.Fatalf("expected 204 without a cookie, got %d", noCookieResponse.Code)
	}
}

func TestCreateUserPasswordResetLink(t *testing.T) {
	server, identity, admin := newFlowTestServer(t)
	created, err := identity.CreateUser(context.Background(), "target@example.com", "target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.CompletePasswordReset(context.Background(), created.RegistrationLink.Token, "target-password"); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.createUserPasswordResetLink(response, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/users/"+created.User.ID.String()+"/password-reset-link", nil), admin, map[string]string{"id": created.User.ID.String()}))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.URL == "" {
		t.Fatalf("expected a reset link URL, got %#v, %v", body, err)
	}

	missing := httptest.NewRecorder()
	server.createUserPasswordResetLink(missing, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/users/missing/password-reset-link", nil), admin, map[string]string{"id": "missing"}))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing user, got %d", missing.Code)
	}
}

func TestListUsersValidatesQueryAndRequiresAdministrator(t *testing.T) {
	server, _, admin := newFlowTestServer(t)

	ok := httptest.NewRecorder()
	server.listUsers(ok, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/users", nil), admin))
	if ok.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", ok.Code, ok.Body.String())
	}

	invalidQuery := httptest.NewRecorder()
	server.listUsers(invalidQuery, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/users?unexpected=1", nil), admin))
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown query parameter, got %d", invalidQuery.Code)
	}

	forbidden := httptest.NewRecorder()
	server.listUsers(forbidden, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/users", nil), &identitydomain.User{ID: foundation.NewID()}))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", forbidden.Code)
	}
}

func TestCreateUserConflictBranches(t *testing.T) {
	server, _, admin := newFlowTestServer(t)
	if _, err := server.identity.CreateUser(context.Background(), "taken@example.com", "taken"); err != nil {
		t.Fatal(err)
	}

	emailTaken := httptest.NewRecorder()
	server.createUser(emailTaken, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/users", strings.NewReader(`{"email":"taken@example.com","username":"other"}`)), admin))
	if emailTaken.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a duplicate email, got %d: %s", emailTaken.Code, emailTaken.Body.String())
	}

	usernameTaken := httptest.NewRecorder()
	server.createUser(usernameTaken, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/users", strings.NewReader(`{"email":"other@example.com","username":"taken"}`)), admin))
	if usernameTaken.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a duplicate username, got %d: %s", usernameTaken.Code, usernameTaken.Body.String())
	}
}

func TestUpdateUserAndReactivateUserErrorBranches(t *testing.T) {
	server, _, admin := newFlowTestServer(t)

	missingUpdate := httptest.NewRecorder()
	server.updateUser(missingUpdate, withUserAndParams(httptest.NewRequest(http.MethodPatch, "http://grom/api/v1/users/missing", strings.NewReader(`{"username":"new"}`)), admin, map[string]string{"id": "missing"}))
	if missingUpdate.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for updating a missing user, got %d: %s", missingUpdate.Code, missingUpdate.Body.String())
	}

	invalidUpdate := httptest.NewRecorder()
	server.updateUser(invalidUpdate, withUserAndParams(httptest.NewRequest(http.MethodPatch, "http://grom/api/v1/users/"+admin.ID.String(), strings.NewReader(`{"username":"   "}`)), admin, map[string]string{"id": admin.ID.String()}))
	if invalidUpdate.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a blank username, got %d: %s", invalidUpdate.Code, invalidUpdate.Body.String())
	}

	created, err := server.identity.CreateUser(context.Background(), "target@example.com", "target")
	if err != nil {
		t.Fatal(err)
	}
	usernameTaken := httptest.NewRecorder()
	server.updateUser(usernameTaken, withUserAndParams(httptest.NewRequest(http.MethodPatch, "http://grom/api/v1/users/"+created.User.ID.String(), strings.NewReader(`{"username":"admin"}`)), admin, map[string]string{"id": created.User.ID.String()}))
	if usernameTaken.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a duplicate username, got %d: %s", usernameTaken.Code, usernameTaken.Body.String())
	}

	missingReactivate := httptest.NewRecorder()
	server.reactivateUser(missingReactivate, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/users/missing/reactivate", nil), admin, map[string]string{"id": "missing"}))
	if missingReactivate.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for reactivating a missing user, got %d", missingReactivate.Code)
	}
}

func TestPromoteUserToSystemViewerErrorBranches(t *testing.T) {
	server, _, admin := newFlowTestServer(t)

	missing := httptest.NewRecorder()
	server.promoteUserToSystemViewer(missing, withUserAndParams(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/users/missing/viewer", nil), admin, map[string]string{"id": "missing"}))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing user, got %d", missing.Code)
	}

	adminAsViewer := httptest.NewRecorder()
	server.promoteUserToSystemViewer(adminAsViewer, withUserAndParams(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/users/"+admin.ID.String()+"/viewer", nil), admin, map[string]string{"id": admin.ID.String()}))
	if adminAsViewer.Code != http.StatusInternalServerError {
		t.Fatalf("expected an administrator to be rejected for viewer promotion, got %d: %s", adminAsViewer.Code, adminAsViewer.Body.String())
	}
}

func TestPromoteUserToSystemAdminMissingUser(t *testing.T) {
	server, _, admin := newFlowTestServer(t)
	response := httptest.NewRecorder()

	server.promoteUserToSystemAdmin(response, withUserAndParams(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/users/missing/administrator", nil), admin, map[string]string{"id": "missing"}))

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestServiceAccountHandlersFullLifecycle(t *testing.T) {
	server, _, admin := newFlowTestServer(t)

	list := httptest.NewRecorder()
	server.listServiceAccounts(list, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/service-accounts", nil), admin))
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
	}

	invalidStatus := httptest.NewRecorder()
	server.listServiceAccounts(invalidStatus, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/service-accounts?status=bogus", nil), admin))
	if invalidStatus.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid status, got %d", invalidStatus.Code)
	}

	createResponse := httptest.NewRecorder()
	server.createServiceAccount(createResponse, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/service-accounts", strings.NewReader(`{"name":"CI","username":"ci","description":""}`)), admin))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var account identitydomain.ServiceAccount
	if err := json.NewDecoder(createResponse.Body).Decode(&account); err != nil {
		t.Fatal(err)
	}

	invalidCreate := httptest.NewRecorder()
	server.createServiceAccount(invalidCreate, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/service-accounts", strings.NewReader(`{"name":"","username":""}`)), admin))
	if invalidCreate.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing name/username, got %d", invalidCreate.Code)
	}

	tokensEmpty := httptest.NewRecorder()
	server.listServiceAccountTokens(tokensEmpty, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/service-accounts/"+account.ID.String()+"/tokens", nil), admin, map[string]string{"id": account.ID.String()}))
	if tokensEmpty.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", tokensEmpty.Code, tokensEmpty.Body.String())
	}

	invalidToken := httptest.NewRecorder()
	server.createServiceAccountToken(invalidToken, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/service-accounts/"+account.ID.String()+"/tokens", strings.NewReader(`{"name":""}`)), admin, map[string]string{"id": account.ID.String()}))
	if invalidToken.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a blank token name, got %d", invalidToken.Code)
	}

	tokenResponse := httptest.NewRecorder()
	server.createServiceAccountToken(tokenResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/service-accounts/"+account.ID.String()+"/tokens", strings.NewReader(`{"name":"pipeline"}`)), admin, map[string]string{"id": account.ID.String()}))
	if tokenResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var created identityapp.CreatedToken
	if err := json.NewDecoder(tokenResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	revokeMissing := httptest.NewRecorder()
	server.revokeServiceAccountToken(revokeMissing, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/service-accounts/x/tokens/missing", nil), admin, map[string]string{"id": account.ID.String(), "tokenId": "missing"}))
	if revokeMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for revoking a missing token, got %d", revokeMissing.Code)
	}

	revoke := httptest.NewRecorder()
	server.revokeServiceAccountToken(revoke, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/service-accounts/x/tokens/y", nil), admin, map[string]string{"id": account.ID.String(), "tokenId": created.Token.ID.String()}))
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", revoke.Code)
	}

	deleteMissing := httptest.NewRecorder()
	server.deleteServiceAccount(deleteMissing, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/service-accounts/missing", nil), admin, map[string]string{"id": "missing"}))
	if deleteMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabling a missing service account, got %d", deleteMissing.Code)
	}

	deleteResponse := httptest.NewRecorder()
	server.deleteServiceAccount(deleteResponse, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/service-accounts/"+account.ID.String(), nil), admin, map[string]string{"id": account.ID.String()}))
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteResponse.Code)
	}
}

func TestProjectHandlersWithoutInventory(t *testing.T) {
	server, _, admin := newFlowTestServer(t)

	createResponse := httptest.NewRecorder()
	server.createProject(createResponse, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects", strings.NewReader(`{"name":"Payments","slug":"payments"}`)), admin))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createResponse.Code, createResponse.Body.String())
	}

	list := httptest.NewRecorder()
	server.listProjects(list, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects", nil), admin))
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"status":"unavailable"`) {
		t.Fatalf("expected the accounted usage to report unavailable without an inventory service, got %s", list.Body.String())
	}

	get := httptest.NewRecorder()
	server.getProject(get, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments", nil), admin, map[string]string{"project": "payments"}))
	if get.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", get.Code, get.Body.String())
	}

	missing := httptest.NewRecorder()
	server.getProject(missing, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/missing", nil), admin, map[string]string{"project": "missing"}))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing project, got %d", missing.Code)
	}
}

func TestCreateSessionIsRateLimitedAfterRepeatedFailures(t *testing.T) {
	server, _, _ := newFlowTestServer(t)

	for i := 0; i < 10; i++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "http://grom/api/v1/session", strings.NewReader(`{"email":"admin@example.com","password":"wrong"}`))
		request.RemoteAddr = "203.0.113.9:5555"
		server.createSession(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i, response.Code)
		}
	}

	blocked := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodPost, "http://grom/api/v1/session", strings.NewReader(`{"email":"admin@example.com","password":"wrong"}`))
	blockedRequest.RemoteAddr = "203.0.113.9:5555"
	server.createSession(blocked, blockedRequest)

	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d: %s", blocked.Code, blocked.Body.String())
	}
	if blocked.Header().Get("Retry-After") == "" {
		t.Fatal("expected a Retry-After header on the rate-limited response")
	}
}

func TestAPIDocsServesHTML(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()

	server.apiDocs(response, httptest.NewRequest(http.MethodGet, "http://grom/api/docs", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Grom API") {
		t.Fatalf("expected the docs page to mention Grom API, got %s", response.Body.String())
	}
}

func TestStringValue(t *testing.T) {
	if got := stringValue(nil); got != "" {
		t.Fatalf("expected an empty string for a nil pointer, got %q", got)
	}
	value := "hello"
	if got := stringValue(&value); got != "hello" {
		t.Fatalf("expected the dereferenced value, got %q", got)
	}
}
