package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAddSystemViewerColumn(t *testing.T) {
	ctx := context.Background()
	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		assertSystemViewerColumn(t, ctx, db)
	})

	t.Run("postgres", func(t *testing.T) {
		db := openPostgresMigrationTestDB(t, ctx)
		if _, err := db.ExecContext(ctx, "CREATE TEMP TABLE users (id TEXT PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		assertSystemViewerColumn(t, ctx, db)
	})
}

func assertSystemViewerColumn(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	if err := addSystemViewerColumn(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id) VALUES ('viewer')"); err != nil {
		t.Fatal(err)
	}
	var viewer bool
	if err := db.NewRaw("SELECT is_system_viewer FROM users WHERE id = 'viewer'").Scan(ctx, &viewer); err != nil {
		t.Fatal(err)
	}
	if viewer {
		t.Fatal("expected the system viewer flag to default to false")
	}
}
