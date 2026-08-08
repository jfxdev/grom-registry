package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAddPasswordResetPurposeColumn(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, "CREATE TABLE password_reset_tokens (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := addPasswordResetPurposeColumn(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO password_reset_tokens (id) VALUES ('reset')"); err != nil {
		t.Fatal(err)
	}
	var purpose string
	if err := db.NewRaw("SELECT purpose FROM password_reset_tokens WHERE id = 'reset'").Scan(ctx, &purpose); err != nil {
		t.Fatal(err)
	}
	if purpose != "password_reset" {
		t.Fatalf("purpose = %q", purpose)
	}
}
