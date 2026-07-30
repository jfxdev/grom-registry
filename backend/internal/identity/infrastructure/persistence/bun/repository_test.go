package bunstore

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
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
			Where("id IN (?)", bun.In([]string{active.ID.String(), disabled.ID.String()})).Exec(context.Background())
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
