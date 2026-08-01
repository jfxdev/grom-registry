package bunstore

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

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

func TestDisableUserRevokesSessionsAtomically(t *testing.T) {
	ctx := context.Background()
	db := openSQLiteRepositoryTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, username TEXT NOT NULL, password_hash TEXT NOT NULL, is_system_admin BOOLEAN NOT NULL, created_at TIMESTAMP NOT NULL, disabled_at TIMESTAMP NULL)`,
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
			used_at TIMESTAMP NULL
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
