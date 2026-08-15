package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAddRegistryStorageAccounting(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			t.Fatal(err)
		}
		assertRegistryStorageAccountingMigration(t, ctx, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openPostgresMigrationTestDB(t, ctx)
		if _, err := db.ExecContext(ctx, "SET search_path TO pg_temp"); err != nil {
			t.Fatal(err)
		}
		assertRegistryStorageAccountingMigration(t, ctx, db)
	})
}

func assertRegistryStorageAccountingMigration(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	for _, statement := range []string{
		"CREATE TABLE projects (id TEXT PRIMARY KEY)",
		"CREATE TABLE registry_repositories (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE)",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := addRegistryStorageAccounting(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO projects (id) VALUES ('project')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO registry_repositories (id, project_id) VALUES ('repository', 'project')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO registry_blob_descriptors (digest, size_bytes, media_type, first_seen_at, last_seen_at) VALUES ('sha256:one', 1, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO registry_manifest_blob_references (repository_id, manifest_digest, blob_digest, role) VALUES ('repository', 'sha256:manifest', 'sha256:one', 'layer')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO registry_manifest_blob_references (repository_id, manifest_digest, blob_digest, role) VALUES ('repository', 'sha256:manifest', 'sha256:one', 'layer')"); err == nil {
		t.Fatal("expected duplicate reference to be rejected")
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM registry_repositories WHERE id = 'repository'"); err != nil {
		t.Fatal(err)
	}
	count, err := db.NewSelect().Table("registry_manifest_blob_references").Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("expected cascading reference deletion, count=%d err=%v", count, err)
	}
}
