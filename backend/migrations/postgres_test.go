package migrations

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func openPostgresMigrationTestDB(t *testing.T, ctx context.Context) *bun.DB {
	t.Helper()
	databaseURL := os.Getenv("GROM_TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("GROM_TEST_POSTGRES_URL is not configured")
	}
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
	db := bun.NewDB(sqlDB, pgdialect.New())
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
