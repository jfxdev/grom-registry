package bunstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestCountActiveServiceAccountAPITokens(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
		account := &identity.ServiceAccount{ID: foundation.NewID(), Name: "Count", Username: "count-" + foundation.NewID().String(), CreatedAt: now}
		other := &identity.ServiceAccount{ID: foundation.NewID(), Name: "Other", Username: "other-" + foundation.NewID().String(), CreatedAt: now}
		for _, candidate := range []*identity.ServiceAccount{account, other} {
			if err := repository.CreateServiceAccount(ctx, candidate); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			_, _ = db.NewDelete().Model((*apiTokenModel)(nil)).Where("principal_id IN (?)", bun.List([]string{account.ID.String(), other.ID.String()})).Exec(context.Background())
			_, _ = db.NewDelete().Model((*serviceAccountModel)(nil)).Where("id IN (?)", bun.List([]string{account.ID.String(), other.ID.String()})).Exec(context.Background())
		})
		insert := func(principalID foundation.ID, revokedAt, expiresAt *time.Time) {
			t.Helper()
			_, err := db.NewInsert().Model(&apiTokenModel{
				ID: foundation.NewID().String(), PublicID: foundation.NewID().String(),
				PrincipalKind: constants.PrincipalServiceAccount, PrincipalID: principalID.String(),
				Name: "key", SecretHash: "hash", CreatedAt: now, RevokedAt: revokedAt, ExpiresAt: expiresAt,
			}).Exec(ctx)
			if err != nil {
				t.Fatal(err)
			}
		}
		assertCount := func(want int) {
			t.Helper()
			got, err := repository.CountActiveServiceAccountAPITokens(ctx, account.ID, now)
			if err != nil || got != want {
				t.Fatalf("active token count = %d, %v; want %d", got, err, want)
			}
		}

		assertCount(0)
		insert(account.ID, nil, nil)
		assertCount(1)
		revoked := now.Add(-time.Minute)
		insert(account.ID, &revoked, nil)
		assertCount(1)
		expired := now.Add(-time.Nanosecond)
		insert(account.ID, nil, &expired)
		assertCount(1)
		insert(account.ID, nil, &now)
		assertCount(1)
		insert(other.ID, nil, nil)
		assertCount(1)
	})
}

func TestCreateServiceAccountAPITokenLimitIsAtomicAcrossServices(t *testing.T) {
	forEachIdentityRepositoryDatabase(t, func(t *testing.T, ctx context.Context, db *bun.DB) {
		repository := New(db)
		now := time.Now().UTC()
		account := &identity.ServiceAccount{ID: foundation.NewID(), Name: "Concurrent", Username: "concurrent-" + foundation.NewID().String(), CreatedAt: now}
		if err := repository.CreateServiceAccount(ctx, account); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.NewDelete().Model((*apiTokenModel)(nil)).Where("principal_id = ?", account.ID.String()).Exec(context.Background())
			_, _ = db.NewDelete().Model((*serviceAccountModel)(nil)).Where("id = ?", account.ID.String()).Exec(context.Background())
		})
		for index := 0; index < constants.MaxActiveServiceAccountAccessKeys-1; index++ {
			token := &identity.APIToken{ID: foundation.NewID(), PublicID: foundation.NewID().String(), ServiceAccountID: account.ID, Name: "existing", SecretHash: "hash", CreatedAt: now}
			if err := repository.CreateServiceAccountAPIToken(ctx, token, now, constants.MaxActiveServiceAccountAccessKeys); err != nil {
				t.Fatal(err)
			}
		}

		barrier := &tokenCreateBarrierRepository{Repository: repository, ready: new(sync.WaitGroup), release: make(chan struct{})}
		barrier.ready.Add(2)
		services := []*identityapp.Service{identityapp.New(barrier, time.Hour), identityapp.New(barrier, time.Hour)}
		results := make(chan error, len(services))
		for index, service := range services {
			go func(index int, service *identityapp.Service) {
				_, err := service.CreateServiceAccountAPIToken(ctx, account.ID, fmt.Sprintf("candidate-%d", index), nil)
				results <- err
			}(index, service)
		}
		barrier.ready.Wait()
		close(barrier.release)

		var created, limited int
		for range services {
			err := <-results
			switch {
			case err == nil:
				created++
			case errors.Is(err, identity.ErrServiceAccountAccessKeyLimit):
				limited++
			default:
				t.Fatalf("unexpected concurrent token creation error: %v", err)
			}
		}
		if created != 1 || limited != 1 {
			t.Fatalf("concurrent token results created=%d limited=%d, want 1/1", created, limited)
		}
		count, err := repository.CountActiveServiceAccountAPITokens(ctx, account.ID, time.Now().UTC())
		if err != nil || count != constants.MaxActiveServiceAccountAccessKeys {
			t.Fatalf("active token count = %d, %v", count, err)
		}
	})
}

type tokenCreateBarrierRepository struct {
	identity.Repository
	ready   *sync.WaitGroup
	release chan struct{}
}

func (r *tokenCreateBarrierRepository) CreateServiceAccountAPIToken(ctx context.Context, token *identity.APIToken, now time.Time, maxActive int) error {
	r.ready.Done()
	<-r.release
	return r.Repository.CreateServiceAccountAPIToken(ctx, token, now, maxActive)
}

func forEachIdentityRepositoryDatabase(t *testing.T, run func(*testing.T, context.Context, *bun.DB)) {
	t.Helper()
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		db, kind, err := database.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "identity.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
			t.Fatal(err)
		}
		run(t, ctx, db)
	})
	t.Run("postgres", func(t *testing.T) {
		databaseURL := os.Getenv("GROM_TEST_POSTGRES_URL")
		if databaseURL == "" {
			t.Skip("GROM_TEST_POSTGRES_URL is not configured")
		}
		db, kind, err := database.Open(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
			t.Fatal(err)
		}
		run(t, ctx, db)
	})
}

func TestListServiceAccountsCanIncludeDisabledAccounts(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		db := openSQLiteRepositoryTestDB(t)
		if _, err := db.ExecContext(ctx, `CREATE TABLE service_accounts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			username TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			disabled_at TIMESTAMP NULL
		)`); err != nil {
			t.Fatal(err)
		}
		assertListServiceAccountsCanIncludeDisabled(t, ctx, db)
	})

	t.Run("postgres", func(t *testing.T) {
		databaseURL := os.Getenv("GROM_TEST_POSTGRES_URL")
		if databaseURL == "" {
			t.Skip("GROM_TEST_POSTGRES_URL is not configured")
		}
		db, kind, err := database.Open(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
			t.Fatal(err)
		}
		assertListServiceAccountsCanIncludeDisabled(t, ctx, db)
	})
}

func TestAdministrativePagesFilterAndAdvanceWithKeysets(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		db := openSQLiteRepositoryTestDB(t)
		for _, statement := range []string{
			`CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, username TEXT NOT NULL, password_hash TEXT NOT NULL, is_system_admin BOOLEAN NOT NULL, is_system_viewer BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMP NOT NULL, disabled_at TIMESTAMP NULL)`,
			`CREATE TABLE service_accounts (id TEXT PRIMARY KEY, name TEXT NOT NULL, username TEXT NOT NULL, description TEXT NOT NULL, created_at TIMESTAMP NOT NULL, disabled_at TIMESTAMP NULL)`,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		assertAdministrativePagesFilterAndAdvanceWithKeysets(t, ctx, db)
	})

	t.Run("postgres", func(t *testing.T) {
		databaseURL := os.Getenv("GROM_TEST_POSTGRES_URL")
		if databaseURL == "" {
			t.Skip("GROM_TEST_POSTGRES_URL is not configured")
		}
		db, kind, err := database.Open(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
			t.Fatal(err)
		}
		assertAdministrativePagesFilterAndAdvanceWithKeysets(t, ctx, db)
	})
}

func assertAdministrativePagesFilterAndAdvanceWithKeysets(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	repository := New(db)
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	userOneID := foundation.NewID()
	userTwoID := foundation.NewID()
	activeID := foundation.NewID()
	disabledID := foundation.NewID()
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*userModel)(nil)).Where("id IN (?)", bun.List([]string{userOneID.String(), userTwoID.String()})).Exec(context.Background())
		_, _ = db.NewDelete().Model((*serviceAccountModel)(nil)).Where("id IN (?)", bun.List([]string{activeID.String(), disabledID.String()})).Exec(context.Background())
	})
	for i, user := range []identity.User{
		{ID: userOneID, Email: userOneID.String() + "@example.com", Username: "alex-" + userOneID.String(), PasswordHash: "hash", CreatedAt: base},
		{ID: userTwoID, Email: userTwoID.String() + "@example.com", Username: "sam-" + userTwoID.String(), PasswordHash: "hash", CreatedAt: base.Add(time.Minute)},
	} {
		if err := repository.CreateUser(ctx, &user); err != nil {
			t.Fatalf("user %d: %v", i, err)
		}
	}
	first, err := repository.ListUsersPage(ctx, "example", foundation.PageRequest{Limit: 1, Scope: "users:q=example"})
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != userTwoID || first.NextCursor == "" {
		t.Fatalf("first user page: %#v, %v", first, err)
	}
	second, err := repository.ListUsersPage(ctx, "example", foundation.PageRequest{Limit: 1, Scope: "users:q=example", Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != userOneID {
		t.Fatalf("second user page: %#v, %v", second, err)
	}

	active := &identity.ServiceAccount{ID: activeID, Name: "Build", Username: "build-" + activeID.String(), Description: "CI", CreatedAt: base}
	disabled := &identity.ServiceAccount{ID: disabledID, Name: "Mirror", Username: "mirror-" + disabledID.String(), Description: "sync", CreatedAt: base.Add(time.Minute)}
	if err := repository.CreateServiceAccount(ctx, active); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateServiceAccount(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	if err := repository.DisableServiceAccount(ctx, disabled.ID); err != nil {
		t.Fatal(err)
	}
	accounts, err := repository.ListServiceAccountsPage(ctx, "mirror", "disabled", foundation.PageRequest{Limit: 1, Scope: "service-accounts:status=disabled:q=mirror"})
	if err != nil || len(accounts.Items) != 1 || accounts.Items[0].ID != disabled.ID {
		t.Fatalf("filtered accounts: %#v, %v", accounts, err)
	}
}

func TestDisableUserRevokesSessionsAtomically(t *testing.T) {
	ctx := context.Background()
	db := openSQLiteRepositoryTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, username TEXT NOT NULL, password_hash TEXT NOT NULL, is_system_admin BOOLEAN NOT NULL, is_system_viewer BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMP NOT NULL, disabled_at TIMESTAMP NULL)`,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, public_id TEXT NOT NULL UNIQUE, user_id TEXT NOT NULL, secret_hash TEXT NOT NULL, created_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	repository := New(db)
	userID := foundation.NewID()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, username, password_hash, is_system_admin, created_at) VALUES (?, ?, ?, ?, ?, ?)`, userID.String(), "user@example.com", "user", "hash", false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions (id, public_id, user_id, secret_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`, foundation.NewID().String(), "session", userID.String(), "hash", time.Now().UTC(), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := repository.DisableUser(ctx, userID); err != nil {
		t.Fatal(err)
	}
	var disabledAt *time.Time
	if err := db.NewSelect().Table("users").Column("disabled_at").Where("id = ?", userID.String()).Scan(ctx, &disabledAt); err != nil {
		t.Fatal(err)
	}
	if disabledAt == nil {
		t.Fatal("expected user to be disabled")
	}
	count, err := db.NewSelect().Table("sessions").Where("user_id = ?", userID.String()).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected sessions to be revoked, got %d", count)
	}

	adminOne, adminTwo := foundation.NewID(), foundation.NewID()
	for _, adminID := range []foundation.ID{adminOne, adminTwo} {
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, username, password_hash, is_system_admin, created_at) VALUES (?, ?, ?, ?, ?, ?)`, adminID.String(), adminID.String()+"@example.com", adminID.String(), "hash", true, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	if err := repository.DisableUser(ctx, adminOne); err != nil {
		t.Fatalf("expected one of two administrators to be disableable: %v", err)
	}
	if err := repository.DisableUser(ctx, adminTwo); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected last administrator protection, got %v", err)
	}
}

func TestPromoteUserToSystemAdmin(t *testing.T) {
	ctx := context.Background()
	db := openSQLiteRepositoryTestDB(t)
	if _, err := db.ExecContext(ctx, `CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, username TEXT NOT NULL, password_hash TEXT NOT NULL, is_system_admin BOOLEAN NOT NULL, is_system_viewer BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMP NOT NULL, disabled_at TIMESTAMP NULL)`); err != nil {
		t.Fatal(err)
	}
	repository := New(db)
	userID := foundation.NewID()
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, username, password_hash, is_system_admin, created_at) VALUES (?, ?, ?, ?, ?, ?)`, userID.String(), "user@example.com", "user", "hash", false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if err := repository.PromoteUserToSystemAdmin(ctx, userID); err != nil {
		t.Fatal(err)
	}
	user, err := repository.FindUserByID(ctx, userID)
	if err != nil || !user.SystemAdmin {
		t.Fatalf("expected promoted user, got %#v, %v", user, err)
	}
}

func TestDisableUserProtectsLastAdministratorConcurrentlyPostgres(t *testing.T) {
	databaseURL := os.Getenv("GROM_TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("GROM_TEST_POSTGRES_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, kind, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}

	repository := New(db)
	suffix := foundation.NewID().String()
	adminOne := &identity.User{ID: foundation.NewID(), Email: "admin-one-" + suffix + "@example.com", Username: "admin-one-" + suffix, PasswordHash: "hash", SystemAdmin: true, CreatedAt: time.Now().UTC()}
	adminTwo := &identity.User{ID: foundation.NewID(), Email: "admin-two-" + suffix + "@example.com", Username: "admin-two-" + suffix, PasswordHash: "hash", SystemAdmin: true, CreatedAt: time.Now().UTC()}
	for _, admin := range []*identity.User{adminOne, adminTwo} {
		if err := repository.CreateUser(ctx, admin); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*userModel)(nil)).Where("id IN (?)", bun.List([]string{adminOne.ID.String(), adminTwo.ID.String()})).Exec(context.Background())
	})

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, admin := range []*identity.User{adminOne, adminTwo} {
		workers.Add(1)
		go func(id foundation.ID) {
			defer workers.Done()
			<-start
			results <- repository.DisableUser(ctx, id)
		}(admin.ID)
	}
	close(start)
	workers.Wait()
	close(results)

	var success, rejected int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, sql.ErrNoRows):
			rejected++
		default:
			t.Fatalf("unexpected concurrent disable error: %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("expected one successful disable and one rejected disable, got %d/%d", success, rejected)
	}
	active, err := db.NewSelect().Model((*userModel)(nil)).Where("is_system_admin = TRUE").Where("disabled_at IS NULL").Where("id IN (?)", bun.List([]string{adminOne.ID.String(), adminTwo.ID.String()})).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("expected one active administrator, got %d", active)
	}
}

func assertListServiceAccountsCanIncludeDisabled(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	repository := New(db)
	suffix := foundation.NewID().String()
	active := &identity.ServiceAccount{
		ID: foundation.NewID(), Name: "Active CI", Username: "active-" + suffix, CreatedAt: time.Now().UTC(),
	}
	disabled := &identity.ServiceAccount{
		ID: foundation.NewID(), Name: "Disabled CI", Username: "disabled-" + suffix, CreatedAt: active.CreatedAt.Add(time.Second),
	}
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*serviceAccountModel)(nil)).
			Where("id IN (?)", bun.List([]string{active.ID.String(), disabled.ID.String()})).Exec(context.Background())
	})
	if err := repository.CreateServiceAccount(ctx, active); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateServiceAccount(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	if err := repository.DisableServiceAccount(ctx, disabled.ID); err != nil {
		t.Fatal(err)
	}

	activeOnly, err := repository.ListServiceAccounts(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeOnly) != 1 || activeOnly[0].ID != active.ID {
		t.Fatalf("expected only the active service account, got %#v", activeOnly)
	}

	all, err := repository.ListServiceAccounts(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected active and disabled service accounts, got %#v", all)
	}
	if all[1].ID != disabled.ID || all[1].DisabledAt == nil {
		t.Fatalf("expected disabled account state to be preserved, got %#v", all[1])
	}
}

func TestInvalidateEphemeralCredentialsDeletesBothTablesAndRollsBack(t *testing.T) {
	ctx := context.Background()
	db := openSQLiteRepositoryTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY, public_id TEXT NOT NULL UNIQUE, user_id TEXT NOT NULL,
			secret_hash TEXT NOT NULL, created_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE password_reset_tokens (
			id TEXT PRIMARY KEY, public_id TEXT NOT NULL UNIQUE, user_id TEXT NOT NULL,
			secret_hash TEXT NOT NULL, created_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL,
			used_at TIMESTAMP NULL, purpose TEXT NOT NULL DEFAULT 'password_reset'
		)`,
		`INSERT INTO users (id) VALUES ('user')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	insertCredentials := func(suffix string) {
		t.Helper()
		now := time.Now().UTC()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO sessions (id, public_id, user_id, secret_hash, created_at, expires_at)
			 VALUES (?, ?, 'user', 'hash', ?, ?)`,
			"session-"+suffix, "session-public-"+suffix, now, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO password_reset_tokens (id, public_id, user_id, secret_hash, created_at, expires_at)
			 VALUES (?, ?, 'user', 'hash', ?, ?)`,
			"reset-"+suffix, "reset-public-"+suffix, now, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	assertCounts := func(sessions, resets int) {
		t.Helper()
		var sessionCount, resetCount int
		if err := db.NewSelect().Table("sessions").ColumnExpr("count(*)").Scan(ctx, &sessionCount); err != nil {
			t.Fatal(err)
		}
		if err := db.NewSelect().Table("password_reset_tokens").ColumnExpr("count(*)").Scan(ctx, &resetCount); err != nil {
			t.Fatal(err)
		}
		if sessionCount != sessions || resetCount != resets {
			t.Fatalf("unexpected credential counts: sessions=%d resets=%d", sessionCount, resetCount)
		}
	}

	repository := New(db)
	insertCredentials("success")
	if err := repository.InvalidateEphemeralCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	assertCounts(0, 0)

	insertCredentials("rollback")
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER reject_reset_delete
		BEFORE DELETE ON password_reset_tokens BEGIN SELECT RAISE(ABORT, 'reject delete'); END`); err != nil {
		t.Fatal(err)
	}
	if err := repository.InvalidateEphemeralCredentials(ctx); err == nil {
		t.Fatal("expected password-reset deletion failure")
	}
	assertCounts(1, 1)
	if _, err := db.ExecContext(ctx, `DROP TRIGGER reject_reset_delete`); err != nil {
		t.Fatal(err)
	}
	if err := repository.InvalidateEphemeralCredentials(ctx); err != nil {
		t.Fatal(err)
	}

	insertCredentials("session-rollback")
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER reject_session_delete
		BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'reject delete'); END`); err != nil {
		t.Fatal(err)
	}
	if err := repository.InvalidateEphemeralCredentials(ctx); err == nil {
		t.Fatal("expected session deletion failure")
	}
	assertCounts(1, 1)
}

func openSQLiteRepositoryTestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}
