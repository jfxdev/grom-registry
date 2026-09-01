package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

func TestEnsureManifestLastPushedByColumn(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(ctx, "CREATE TABLE registry_manifests (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		assertManifestLastPushedByColumn(t, ctx, db)
	})

	t.Run("postgres", func(t *testing.T) {
		db := openPostgresMigrationTestDB(t, ctx)
		if _, err := db.ExecContext(ctx, "SET search_path TO pg_temp"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TEMP TABLE registry_manifests (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		assertManifestLastPushedByColumn(t, ctx, db)
	})
}

func TestManifestLastPushedByMigrationCannotBeUnapplied(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := migrate.NewMigrator(db, Collection, migrate.WithMarkAppliedOnSuccess(true))
	if err := migrator.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Rollback(ctx); err == nil || !strings.Contains(err.Error(), "cannot be rolled back") {
		t.Fatalf("expected migration rollback to be refused, got %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migration reran after failed rollback: %v", err)
	}
}

func assertManifestLastPushedByColumn(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	if err := ensureManifestLastPushedByColumn(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ensureManifestLastPushedByColumn(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO registry_manifests (id) VALUES ('manifest')"); err != nil {
		t.Fatal(err)
	}
	var pushedBy string
	if err := db.NewRaw("SELECT last_pushed_by FROM registry_manifests WHERE id = 'manifest'").Scan(ctx, &pushedBy); err != nil {
		t.Fatal(err)
	}
	if pushedBy != "" {
		t.Fatalf("expected empty last_pushed_by default, got %q", pushedBy)
	}
}
