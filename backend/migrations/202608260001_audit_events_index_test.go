package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAddAuditEventsIndex(t *testing.T) {
	ctx := context.Background()

	const createAuditEvents = `CREATE TABLE audit_events (
		id TEXT PRIMARY KEY,
		actor_kind TEXT NOT NULL,
		actor_id TEXT NOT NULL,
		action TEXT NOT NULL,
		resource_kind TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL
	)`

	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(ctx, createAuditEvents); err != nil {
			t.Fatal(err)
		}
		if err := addAuditEventsIndex(ctx, db); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.NewRaw(
			"SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
			"idx_audit_events_created_at_id",
		).Scan(ctx, &count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected audit-events index to exist, got %d", count)
		}
		if err := dropAuditEventsIndex(ctx, db); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("postgres", func(t *testing.T) {
		db := openPostgresMigrationTestDB(t, ctx)
		if _, err := db.ExecContext(ctx, "SET search_path TO pg_temp"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TEMP TABLE audit_events "+
			"(id TEXT PRIMARY KEY, actor_kind TEXT NOT NULL, actor_id TEXT NOT NULL, "+
			"action TEXT NOT NULL, resource_kind TEXT NOT NULL, resource_id TEXT NOT NULL, "+
			"metadata_json TEXT NOT NULL DEFAULT '{}', created_at TIMESTAMP NOT NULL)"); err != nil {
			t.Fatal(err)
		}
		if err := addAuditEventsIndex(ctx, db); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.NewRaw(
			"SELECT count(*) FROM pg_indexes WHERE indexname = ?",
			"idx_audit_events_created_at_id",
		).Scan(ctx, &count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected audit-events index to exist, got %d", count)
		}
	})
}
