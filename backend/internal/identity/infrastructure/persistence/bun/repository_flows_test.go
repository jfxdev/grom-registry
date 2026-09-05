package bunstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/uptrace/bun"
)

func newTestUser(t *testing.T, ctx context.Context, repository *Repository, disabled bool) *identity.User {
	t.Helper()
	id := foundation.NewID()
	now := time.Now().UTC()
	user := &identity.User{
		ID: id, Email: id.String() + "@example.com", Username: "user-" + id.String(),
		PasswordHash: "hash", CreatedAt: now,
	}
	if disabled {
		user.DisabledAt = &now
	}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestCountUsersAndListUsers(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		if count, err := repository.CountUsers(ctx); err != nil || count != 0 {
			t.Fatalf("expected zero users initially, got %d, %v", count, err)
		}
		newTestUser(t, ctx, repository, false)
		newTestUser(t, ctx, repository, false)

		count, err := repository.CountUsers(ctx)
		if err != nil || count != 2 {
			t.Fatalf("expected two users, got %d, %v", count, err)
		}
		users, err := repository.ListUsers(ctx)
		if err != nil || len(users) != 2 {
			t.Fatalf("expected two listed users, got %#v, %v", users, err)
		}
	})
}

func TestCreateUserWithPasswordRegistrationReset(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		id := foundation.NewID()
		now := time.Now().UTC()
		user := &identity.User{ID: id, Email: id.String() + "@example.com", Username: "user-" + id.String(), PasswordHash: "hash", CreatedAt: now, DisabledAt: &now}
		reset := &identity.PasswordReset{
			ID: foundation.NewID(), PublicID: "public-" + id.String(), UserID: id, SecretHash: "secret-hash",
			CreatedAt: now, ExpiresAt: now.Add(time.Hour), Purpose: identity.PasswordResetPurposeRegistration,
		}
		if err := repository.CreateUserWithPasswordReset(ctx, user, reset); err != nil {
			t.Fatal(err)
		}

		found, err := repository.FindUserByEmail(ctx, user.Email)
		if err == nil || found != nil {
			t.Fatalf("expected the registered user to be disabled and unfindable by email, got %#v, %v", found, err)
		}
		if _, err := repository.FindUserByUsername(ctx, user.Username); err == nil {
			t.Fatal("expected the disabled user to be unfindable by username")
		}
		reloaded, err := repository.FindUserByIDIncludingDisabled(ctx, id)
		if err != nil || reloaded.DisabledAt == nil {
			t.Fatalf("expected to find the disabled user by ID, got %#v, %v", reloaded, err)
		}

		fetchedReset, err := repository.FindPasswordResetByPublicID(ctx, reset.PublicID)
		if err != nil || fetchedReset.UserID != id {
			t.Fatalf("expected to find the password reset, got %#v, %v", fetchedReset, err)
		}
	})
}

func TestCreateUserWithPasswordResetRejectsDuplicateUsername(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		existing := newTestUser(t, ctx, repository, false)

		id := foundation.NewID()
		now := time.Now().UTC()
		duplicate := &identity.User{ID: id, Email: id.String() + "@example.com", Username: existing.Username, PasswordHash: "hash", CreatedAt: now}
		reset := &identity.PasswordReset{ID: foundation.NewID(), PublicID: "public-" + id.String(), UserID: id, SecretHash: "hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Purpose: identity.PasswordResetPurposeRegistration}

		if err := repository.CreateUserWithPasswordReset(ctx, duplicate, reset); !errors.Is(err, identity.ErrUsernameAlreadyExists) {
			t.Fatalf("expected ErrUsernameAlreadyExists, got %v", err)
		}
	})
}

func TestPromoteUserToSystemViewerAtRepositoryLevel(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		user := newTestUser(t, ctx, repository, false)

		if err := repository.PromoteUserToSystemViewer(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
		found, err := repository.FindUserByID(ctx, user.ID)
		if err != nil || !found.SystemViewer {
			t.Fatalf("expected the user to become a viewer, got %#v, %v", found, err)
		}
		if err := repository.PromoteUserToSystemViewer(ctx, foundation.NewID()); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows for a missing user, got %v", err)
		}
	})
}

func TestUpdateUserPasswordAtRepositoryLevel(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		user := newTestUser(t, ctx, repository, false)

		if err := repository.UpdateUserPassword(ctx, user.ID, "new-hash"); err != nil {
			t.Fatal(err)
		}
		found, err := repository.FindUserByID(ctx, user.ID)
		if err != nil || found.PasswordHash != "new-hash" {
			t.Fatalf("expected the password hash to update, got %#v, %v", found, err)
		}
		if err := repository.UpdateUserPassword(ctx, foundation.NewID(), "new-hash"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows for a missing user, got %v", err)
		}
	})
}

func TestPasswordResetLifecycle(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		user := newTestUser(t, ctx, repository, false)
		now := time.Now().UTC()

		reset := &identity.PasswordReset{
			ID: foundation.NewID(), PublicID: "public-" + user.ID.String(), UserID: user.ID, SecretHash: "secret-hash",
			CreatedAt: now, ExpiresAt: now.Add(time.Hour), Purpose: identity.PasswordResetPurposePasswordReset,
		}
		if err := repository.CreatePasswordReset(ctx, reset); err != nil {
			t.Fatal(err)
		}

		// A second reset for the same user invalidates the first (used_at set).
		second := &identity.PasswordReset{
			ID: foundation.NewID(), PublicID: "public-2-" + user.ID.String(), UserID: user.ID, SecretHash: "secret-hash-2",
			CreatedAt: now.Add(time.Minute), ExpiresAt: now.Add(2 * time.Hour), Purpose: identity.PasswordResetPurposePasswordReset,
		}
		if err := repository.CreatePasswordReset(ctx, second); err != nil {
			t.Fatal(err)
		}
		firstReloaded, err := repository.FindPasswordResetByPublicID(ctx, reset.PublicID)
		if err != nil || firstReloaded.UsedAt == nil {
			t.Fatalf("expected the first reset to be invalidated by the second, got %#v, %v", firstReloaded, err)
		}

		if err := repository.ConsumePasswordReset(ctx, second.ID, user.ID, "new-hash", false, now.Add(3*time.Minute)); err != nil {
			t.Fatal(err)
		}
		updated, err := repository.FindUserByID(ctx, user.ID)
		if err != nil || updated.PasswordHash != "new-hash" {
			t.Fatalf("expected the password hash to update from the reset, got %#v, %v", updated, err)
		}

		if err := repository.ConsumePasswordReset(ctx, second.ID, user.ID, "another-hash", false, now.Add(4*time.Minute)); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected reusing a consumed reset to fail, got %v", err)
		}

		expired := &identity.PasswordReset{
			ID: foundation.NewID(), PublicID: "public-3-" + user.ID.String(), UserID: user.ID, SecretHash: "secret-hash-3",
			CreatedAt: now, ExpiresAt: now.Add(time.Millisecond), Purpose: identity.PasswordResetPurposePasswordReset,
		}
		if err := repository.CreatePasswordReset(ctx, expired); err != nil {
			t.Fatal(err)
		}
		if err := repository.ConsumePasswordReset(ctx, expired.ID, user.ID, "yet-another-hash", false, now.Add(time.Hour)); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected an expired reset to fail, got %v", err)
		}
	})
}

func TestConsumePasswordResetActivatesDisabledRegistration(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		user := newTestUser(t, ctx, repository, true)
		now := time.Now().UTC()
		reset := &identity.PasswordReset{
			ID: foundation.NewID(), PublicID: "public-" + user.ID.String(), UserID: user.ID, SecretHash: "secret-hash",
			CreatedAt: now, ExpiresAt: now.Add(time.Hour), Purpose: identity.PasswordResetPurposeRegistration,
		}
		if err := repository.CreatePasswordReset(ctx, reset); err != nil {
			t.Fatal(err)
		}

		if err := repository.ConsumePasswordReset(ctx, reset.ID, user.ID, "new-hash", true, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		activated, err := repository.FindUserByID(ctx, user.ID)
		if err != nil || activated.DisabledAt != nil {
			t.Fatalf("expected registration completion to activate the user, got %#v, %v", activated, err)
		}
	})
}

func TestSessionLifecycle(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		user := newTestUser(t, ctx, repository, false)
		now := time.Now().UTC()
		session := &identity.Session{
			ID: foundation.NewID(), PublicID: "public-" + user.ID.String(), UserID: user.ID,
			SecretHash: "secret-hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}
		if err := repository.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}

		found, err := repository.FindSessionByPublicID(ctx, session.PublicID)
		if err != nil || found.UserID != user.ID {
			t.Fatalf("expected to find the session, got %#v, %v", found, err)
		}

		if err := repository.DeleteSession(ctx, session.PublicID); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.FindSessionByPublicID(ctx, session.PublicID); err == nil {
			t.Fatal("expected the session to be gone after deletion")
		}
	})
}

func TestFindServiceAccountByUsername(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		id := foundation.NewID()
		account := &identity.ServiceAccount{ID: id, Name: "CI", Username: "ci-" + id.String(), CreatedAt: time.Now().UTC()}
		if err := repository.CreateServiceAccount(ctx, account); err != nil {
			t.Fatal(err)
		}

		found, err := repository.FindServiceAccountByUsername(ctx, account.Username)
		if err != nil || found.ID != id {
			t.Fatalf("expected to find the service account, got %#v, %v", found, err)
		}
		if _, err := repository.FindServiceAccountByUsername(ctx, "missing-account"); err == nil {
			t.Fatal("expected a missing username to fail")
		}
	})
}

func TestViewerAPITokenLifecycle(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		user := newTestUser(t, ctx, repository, false)
		now := time.Now().UTC()
		token := &identity.APIToken{
			ID: foundation.NewID(), PublicID: "public-" + user.ID.String(),
			Principal: foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: user.ID},
			Name:      "local pull", SecretHash: "secret-hash", CreatedAt: now,
		}
		if err := repository.CreateViewerAPIToken(ctx, token); err != nil {
			t.Fatal(err)
		}

		duplicate := &identity.APIToken{
			ID: foundation.NewID(), PublicID: "public-2-" + user.ID.String(),
			Principal: foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: user.ID},
			Name:      "second token", SecretHash: "secret-hash-2", CreatedAt: now,
		}
		if err := repository.CreateViewerAPIToken(ctx, duplicate); !errors.Is(err, identity.ErrViewerRegistryTokenAlreadyExists) {
			t.Fatalf("expected ErrViewerRegistryTokenAlreadyExists for a second active token, got %v", err)
		}

		tokens, err := repository.ListViewerAPITokens(ctx, user.ID)
		if err != nil || len(tokens) != 1 {
			t.Fatalf("expected one listed viewer token, got %#v, %v", tokens, err)
		}

		found, err := repository.FindAPITokenByPublicID(ctx, token.PublicID)
		if err != nil || found.ID != token.ID {
			t.Fatalf("expected to find the token by public ID, got %#v, %v", found, err)
		}

		if err := repository.TouchAPIToken(ctx, token.ID); err != nil {
			t.Fatal(err)
		}

		if err := repository.RevokeViewerAPIToken(ctx, user.ID, token.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.FindAPITokenByPublicID(ctx, token.PublicID); err == nil {
			t.Fatal("expected a revoked token to no longer resolve")
		}
		if err := repository.RevokeViewerAPIToken(ctx, user.ID, token.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected revoking an already-revoked token to fail, got %v", err)
		}

		if err := repository.CreateViewerAPIToken(ctx, &identity.APIToken{
			ID: foundation.NewID(), PublicID: "public-3-" + user.ID.String(),
			Principal: foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: user.ID},
			Name:      "third token", SecretHash: "secret-hash-3", CreatedAt: now,
		}); err != nil {
			t.Fatalf("expected creating a new viewer token after revocation to succeed, got %v", err)
		}
	})
}

func TestServiceAccountAPITokenListingAndRevocation(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		id := foundation.NewID()
		account := &identity.ServiceAccount{ID: id, Name: "CI", Username: "ci-" + id.String(), CreatedAt: time.Now().UTC()}
		if err := repository.CreateServiceAccount(ctx, account); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		token := &identity.APIToken{
			ID: foundation.NewID(), PublicID: "public-" + id.String(), ServiceAccountID: id,
			Name: "pipeline", SecretHash: "secret-hash", CreatedAt: now,
		}
		if err := repository.CreateServiceAccountAPIToken(ctx, token, now, constants.MaxActiveServiceAccountAccessKeys); err != nil {
			t.Fatal(err)
		}

		tokens, err := repository.ListServiceAccountAPITokens(ctx, id)
		if err != nil || len(tokens) != 1 {
			t.Fatalf("expected one listed token, got %#v, %v", tokens, err)
		}

		found, err := repository.FindAPITokenByPublicID(ctx, token.PublicID)
		if err != nil || found.ServiceAccountID != id {
			t.Fatalf("expected to find the token by public ID, got %#v, %v", found, err)
		}

		page, err := repository.ListServiceAccountAPITokensPage(ctx, id, foundation.PageRequest{Limit: 10, Scope: "tokens"})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("expected one paged token, got %#v, %v", page, err)
		}

		if err := repository.RevokeServiceAccountAPIToken(ctx, id, token.ID); err != nil {
			t.Fatal(err)
		}
		if err := repository.RevokeServiceAccountAPIToken(ctx, id, token.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected revoking an already-revoked token to fail, got %v", err)
		}
	})
}
