package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAddManifestPlatforms(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(ctx, "CREATE TABLE registry_manifests (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		assertManifestPlatforms(t, ctx, db)
	})

	t.Run("postgres", func(t *testing.T) {
		db := openPostgresMigrationTestDB(t, ctx)
		if _, err := db.ExecContext(ctx, "SET search_path TO pg_temp"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TEMP TABLE registry_manifests (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		assertManifestPlatforms(t, ctx, db)
	})
}

func assertManifestPlatforms(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	if err := addManifestPlatforms(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO registry_manifests (id) VALUES ('manifest')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO registry_manifest_platforms
		(manifest_id, digest, os, architecture, variant, compressed_size)
		VALUES ('manifest', 'sha256:one', 'linux', 'amd64', '', 1000),
		       ('manifest', 'sha256:two', 'linux', 'amd64', '', 2000)`); err != nil {
		t.Fatalf("same platform coordinates with distinct child digests must be preserved: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO registry_manifest_platforms
		(manifest_id, digest, os, architecture, variant, compressed_size)
		VALUES ('manifest', 'sha256:one', 'linux', 'amd64', '', 1000)`); err == nil {
		t.Fatal("expected duplicate manifest child digest to be rejected")
	}
}
