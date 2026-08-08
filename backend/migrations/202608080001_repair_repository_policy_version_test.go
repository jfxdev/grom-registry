package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestEnsureRepositoryPolicyVersionColumnRepairsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "CREATE TABLE registry_repositories (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepositoryPolicyVersionColumn(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ensureRepositoryPolicyVersionColumn(ctx, db); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.NewRaw("SELECT COUNT(*) FROM pragma_table_info('registry_repositories') WHERE name = 'policy_version'").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one policy_version column, got %d", count)
	}
}
