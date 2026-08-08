package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAddSingleActiveViewerRegistryTokenIndex(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "CREATE TABLE api_tokens (principal_kind TEXT, principal_id TEXT, revoked_at TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if err := addSingleActiveViewerRegistryTokenIndex(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO api_tokens (principal_kind, principal_id) VALUES ('user', 'viewer')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO api_tokens (principal_kind, principal_id) VALUES ('user', 'viewer')"); err == nil {
		t.Fatal("expected a second active viewer token to be rejected")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO api_tokens (principal_kind, principal_id, revoked_at) VALUES ('user', 'viewer', CURRENT_TIMESTAMP)"); err != nil {
		t.Fatalf("expected revoked viewer token to be accepted: %v", err)
	}
}
