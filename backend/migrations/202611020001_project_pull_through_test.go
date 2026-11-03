package migrations

import (
	"context"
	"testing"

	"database/sql"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestEnsureProjectPullThroughColumns(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() { _ = db.Close() })
		createLegacyProjectsTable(t, ctx, db)
		assertProjectPullThroughColumns(t, ctx, db)
	})

	t.Run("postgres", func(t *testing.T) {
		db := openPostgresMigrationTestDB(t, ctx)
		if _, err := db.ExecContext(ctx, "SET search_path TO pg_temp"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TEMP TABLE projects (id TEXT PRIMARY KEY, slug TEXT NOT NULL, name TEXT NOT NULL, created_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL)"); err != nil {
			t.Fatal(err)
		}
		assertProjectPullThroughColumns(t, ctx, db)
	})
}

func createLegacyProjectsTable(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, "CREATE TABLE projects (id TEXT PRIMARY KEY, slug TEXT NOT NULL, name TEXT NOT NULL, created_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL)"); err != nil {
		t.Fatal(err)
	}
}

func assertProjectPullThroughColumns(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES ('project', 'docker', 'Docker', 'admin', CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if err := ensureProjectPullThroughColumns(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ensureProjectPullThroughColumns(ctx, db); err != nil {
		t.Fatal(err)
	}
	var mode, upstream string
	if err := db.NewRaw("SELECT mode, upstream FROM projects WHERE id = 'project'").Scan(ctx, &mode, &upstream); err != nil {
		t.Fatal(err)
	}
	if mode != "standard" || upstream != "" {
		t.Fatalf("expected legacy project defaults, got mode=%q upstream=%q", mode, upstream)
	}
}

func TestProjectPullThroughMigrationFailsWithoutProjectsTable(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	if err := ensureProjectPullThroughColumns(ctx, db); err == nil {
		t.Fatal("migration succeeded without a projects table")
	}
	if err := preventProjectPullThroughRollback(ctx, db); err == nil {
		t.Fatal("pull-through migration rollback was allowed")
	}
}
