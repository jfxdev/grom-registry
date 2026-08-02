package database

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestMigratePropagatesMigrationFailureWithoutMarkingItApplied(t *testing.T) {
	ctx := context.Background()
	db, kind, err := Open(ctx, "sqlite://file:migration-failure-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}

	err = Migrate(ctx, db, kind, time.Second, slog.Default())
	if err == nil {
		t.Fatal("expected migration failure")
	}

	var count int
	if err := db.NewSelect().Table("bun_migrations").ColumnExpr("count(*)").Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed migration was marked as applied: %d", count)
	}
}
