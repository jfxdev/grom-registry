package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
)

func TestBootstrapAdminCreatesFirstAdminAndSkipsLater(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()

	if err := service.BootstrapAdmin(ctx, "Admin@Example.com", "admin", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}
	if len(repository.users) != 1 {
		t.Fatalf("expected exactly one bootstrapped user, got %d", len(repository.users))
	}
	var admin *identity.User
	for _, user := range repository.users {
		admin = user
	}
	if admin.Email != "admin@example.com" || !admin.SystemAdmin {
		t.Fatalf("expected normalized admin user, got %#v", admin)
	}

	if err := service.BootstrapAdmin(ctx, "second@example.com", "second", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}
	if len(repository.users) != 1 {
		t.Fatalf("expected bootstrap to be a no-op once a user exists, got %d users", len(repository.users))
	}
}

func TestLoginAuthenticateSessionAndLogout(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	if err := service.BootstrapAdmin(ctx, "admin@example.com", "admin", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}

	session, user, err := service.Login(ctx, "admin@example.com", "bootstrap-password")
	if err != nil {
		t.Fatal(err)
	}
	if session == "" || user == nil || user.Email != "admin@example.com" {
		t.Fatalf("expected a session and user, got %q %#v", session, user)
	}

	if _, _, err := service.Login(ctx, "admin@example.com", "wrong-password"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated for a wrong password, got %v", err)
	}

	authenticated, err := service.AuthenticateSession(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.Email != "admin@example.com" {
		t.Fatalf("expected the session to resolve to the admin user, got %#v", authenticated)
	}

	if _, err := service.AuthenticateSession(ctx, "not-a-valid-token"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated for a malformed token, got %v", err)
	}
	if _, err := service.AuthenticateSession(ctx, "grms_missing_secret"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated for an unknown session, got %v", err)
	}

	if err := service.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSession(ctx, session); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected the session to be gone after logout, got %v", err)
	}
	if err := service.Logout(ctx, "not-a-valid-token"); err != nil {
		t.Fatalf("expected logout of a malformed token to be a no-op, got %v", err)
	}
}

func TestInvalidateEphemeralCredentials(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)

	if err := service.InvalidateEphemeralCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !repository.invalidated {
		t.Fatal("expected the repository invalidation hook to run")
	}
}

func TestListUsersAndListUsersPage(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	if err := service.BootstrapAdmin(ctx, "admin@example.com", "admin", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}

	users, err := service.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("expected one listed user, got %#v, %v", users, err)
	}

	page, err := service.ListUsersPage(ctx, "", foundation.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("expected one paged user, got %#v, %v", page, err)
	}
}

func TestPromoteUserToSystemViewer(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	member := &identity.User{ID: foundation.NewID(), Email: "member@example.com", Username: "member"}
	if err := repository.CreateUser(ctx, member); err != nil {
		t.Fatal(err)
	}

	promoted, err := service.PromoteUserToSystemViewer(ctx, member.ID)
	if err != nil || !promoted.SystemViewer {
		t.Fatalf("expected the user to become a viewer, got %#v, %v", promoted, err)
	}

	againstIdempotent, err := service.PromoteUserToSystemViewer(ctx, member.ID)
	if err != nil || !againstIdempotent.SystemViewer {
		t.Fatalf("expected promoting an existing viewer to be a no-op, got %#v, %v", againstIdempotent, err)
	}

	admin, err := service.PromoteUserToSystemAdmin(ctx, member.ID)
	if err != nil || !admin.SystemAdmin {
		t.Fatalf("expected promotion to administrator, got %#v, %v", admin, err)
	}
	if _, err := service.PromoteUserToSystemViewer(ctx, member.ID); err == nil {
		t.Fatal("expected an administrator to be rejected for viewer promotion")
	}
}

func TestFindUserAndDisableUser(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	member := &identity.User{ID: foundation.NewID(), Email: "member@example.com", Username: "member"}
	if err := repository.CreateUser(ctx, member); err != nil {
		t.Fatal(err)
	}

	found, err := service.FindUser(ctx, member.ID)
	if err != nil || found.ID != member.ID {
		t.Fatalf("expected to find the user, got %#v, %v", found, err)
	}

	if err := service.DisableUser(ctx, member.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FindUser(ctx, member.ID); err == nil {
		t.Fatal("expected a disabled user to no longer be findable through FindUser")
	}
}

func TestChangePassword(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	if err := service.BootstrapAdmin(ctx, "admin@example.com", "admin", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}
	var adminID foundation.ID
	for id := range repository.users {
		adminID = id
	}

	if err := service.ChangePassword(ctx, adminID, "wrong-password", "new-strong-password"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("expected ErrInvalidCurrentPassword, got %v", err)
	}
	if err := service.ChangePassword(ctx, adminID, "bootstrap-password", "short"); err == nil {
		t.Fatal("expected a too-short new password to be rejected")
	}
	if err := service.ChangePassword(ctx, adminID, "bootstrap-password", "new-strong-password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(ctx, "admin@example.com", "new-strong-password"); err != nil {
		t.Fatalf("expected login with the new password to succeed, got %v", err)
	}
}

func TestCreateAndCompletePasswordReset(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	if err := service.BootstrapAdmin(ctx, "admin@example.com", "admin", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}
	var adminID foundation.ID
	for id := range repository.users {
		adminID = id
	}

	if _, err := service.CreatePasswordReset(ctx, foundation.NewID()); err == nil {
		t.Fatal("expected a reset for a missing user to fail")
	}

	created, err := service.CreatePasswordReset(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.CompletePasswordReset(ctx, created.Token, "short"); err == nil {
		t.Fatal("expected a too-short new password to be rejected")
	}
	if _, err := service.CompletePasswordReset(ctx, "not-a-valid-token", "new-strong-password"); !errors.Is(err, ErrInvalidPasswordReset) {
		t.Fatalf("expected ErrInvalidPasswordReset for a malformed token, got %v", err)
	}

	userID, err := service.CompletePasswordReset(ctx, created.Token, "new-strong-password")
	if err != nil || userID != adminID {
		t.Fatalf("expected the reset to resolve to the admin user, got %q, %v", userID, err)
	}
	if _, err := service.CompletePasswordReset(ctx, created.Token, "new-strong-password"); !errors.Is(err, ErrInvalidPasswordReset) {
		t.Fatalf("expected a reused reset token to fail, got %v", err)
	}
}

func TestCreateServiceAccountRejectsMissingFields(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()

	if _, err := service.CreateServiceAccount(ctx, "", "", ""); err == nil {
		t.Fatal("expected missing name and username to be rejected")
	}
	account, err := service.CreateServiceAccount(ctx, "CI", "ci", "pipeline account")
	if err != nil {
		t.Fatal(err)
	}
	if account.Name != "CI" || account.Username != "ci" {
		t.Fatalf("unexpected created account: %#v", account)
	}
}

func TestListServiceAccountsAndPageAndFindAndDisable(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	account, err := service.CreateServiceAccount(ctx, "CI", "ci", "")
	if err != nil {
		t.Fatal(err)
	}

	accounts, err := service.ListServiceAccounts(ctx, false)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("expected one active account, got %#v, %v", accounts, err)
	}

	page, err := service.ListServiceAccountsPage(ctx, "", "", foundation.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("expected one paged account, got %#v, %v", page, err)
	}

	found, err := service.FindServiceAccount(ctx, account.ID)
	if err != nil || found.ID != account.ID {
		t.Fatalf("expected to find the account, got %#v, %v", found, err)
	}

	if err := service.DisableServiceAccount(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	activeOnly, err := service.ListServiceAccounts(ctx, false)
	if err != nil || len(activeOnly) != 0 {
		t.Fatalf("expected the disabled account to be excluded, got %#v, %v", activeOnly, err)
	}
	includingDisabled, err := service.ListServiceAccounts(ctx, true)
	if err != nil || len(includingDisabled) != 1 {
		t.Fatalf("expected includeDisabled to surface the account, got %#v, %v", includingDisabled, err)
	}
}

func TestServiceAccountAPITokenLifecycle(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	account, err := service.CreateServiceAccount(ctx, "CI", "ci", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.CreateServiceAccountAPIToken(ctx, account.ID, "", nil); err == nil {
		t.Fatal("expected an empty token name to be rejected")
	}
	created, err := service.CreateServiceAccountAPIToken(ctx, account.ID, "pipeline", nil)
	if err != nil {
		t.Fatal(err)
	}

	tokens, err := service.ListServiceAccountAPITokens(ctx, account.ID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("expected one listed token, got %#v, %v", tokens, err)
	}
	if _, err := service.ListServiceAccountAPITokens(ctx, foundation.NewID()); err == nil {
		t.Fatal("expected listing tokens for a missing account to fail")
	}

	page, err := service.ListServiceAccountAPITokensPage(ctx, account.ID, foundation.PageRequest{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("expected one paged token, got %#v, %v", page, err)
	}
	if _, err := service.ListServiceAccountAPITokensPage(ctx, foundation.NewID(), foundation.PageRequest{Limit: 10}); err == nil {
		t.Fatal("expected paging tokens for a missing account to fail")
	}

	active, err := service.CountActiveServiceAccountAPITokens(ctx, account.ID)
	if err != nil || active != 1 {
		t.Fatalf("expected one active token, got %d, %v", active, err)
	}

	if err := service.RevokeServiceAccountAPIToken(ctx, account.ID, created.Token.ID); err != nil {
		t.Fatal(err)
	}
	activeAfterRevoke, err := service.CountActiveServiceAccountAPITokens(ctx, account.ID)
	if err != nil || activeAfterRevoke != 0 {
		t.Fatalf("expected no active tokens after revoke, got %d, %v", activeAfterRevoke, err)
	}
}

func TestListAndRevokeViewerRegistryTokens(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	viewer := &identity.User{ID: foundation.NewID(), Email: "viewer@example.com", Username: "viewer", SystemViewer: true}
	member := &identity.User{ID: foundation.NewID(), Email: "member@example.com", Username: "member"}
	if err := repository.CreateUser(ctx, viewer); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateUser(ctx, member); err != nil {
		t.Fatal(err)
	}

	if _, err := service.ListViewerRegistryTokens(ctx, member.ID); !errors.Is(err, ErrViewerPermissionRequired) {
		t.Fatalf("expected ErrViewerPermissionRequired for a regular user, got %v", err)
	}
	if err := service.RevokeViewerRegistryToken(ctx, member.ID, foundation.NewID()); !errors.Is(err, ErrViewerPermissionRequired) {
		t.Fatalf("expected ErrViewerPermissionRequired for a regular user, got %v", err)
	}

	created, err := service.CreateViewerRegistryToken(ctx, viewer.ID, "local pull", nil)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.ListViewerRegistryTokens(ctx, viewer.ID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("expected one listed viewer token, got %#v, %v", tokens, err)
	}
	if err := service.RevokeViewerRegistryToken(ctx, viewer.ID, created.Token.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticateRegistryServiceAccountAndUnknownPrincipal(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	account, err := service.CreateServiceAccount(ctx, "CI", "ci", "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateServiceAccountAPIToken(ctx, account.ID, "pipeline", nil)
	if err != nil {
		t.Fatal(err)
	}

	principal, err := service.AuthenticateRegistry(ctx, account.Username, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ID != account.ID {
		t.Fatalf("unexpected authenticated principal: %#v", principal)
	}

	if _, err := service.AuthenticateRegistry(ctx, "someone-else", created.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected username mismatch to fail, got %v", err)
	}
	if _, err := service.AuthenticateRegistry(ctx, account.Username, "grm_bogus_secret"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected an unknown token to fail, got %v", err)
	}
	if _, err := service.AuthenticateRegistry(ctx, account.Username, "not-a-valid-token"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected a malformed token to fail, got %v", err)
	}
}
