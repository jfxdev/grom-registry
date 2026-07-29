package bunstore

import (
	"context"
	"database/sql"
	"testing"
	"time"

	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestListServiceAccountsCanIncludeDisabledAccounts(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `CREATE TABLE service_accounts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		username TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		disabled_at TIMESTAMP NULL
	)`); err != nil {
		t.Fatal(err)
	}

	repository := New(db)
	active := &identity.ServiceAccount{
		ID: foundation.NewID(), Name: "Active CI", Username: "active-ci", CreatedAt: time.Now().UTC(),
	}
	disabled := &identity.ServiceAccount{
		ID: foundation.NewID(), Name: "Disabled CI", Username: "disabled-ci", CreatedAt: active.CreatedAt.Add(time.Second),
	}
	if err := repository.CreateServiceAccount(ctx, active); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateServiceAccount(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	if err := repository.DisableServiceAccount(ctx, disabled.ID); err != nil {
		t.Fatal(err)
	}

	activeOnly, err := repository.ListServiceAccounts(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeOnly) != 1 || activeOnly[0].ID != active.ID {
		t.Fatalf("expected only the active service account, got %#v", activeOnly)
	}

	all, err := repository.ListServiceAccounts(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected active and disabled service accounts, got %#v", all)
	}
	if all[1].ID != disabled.ID || all[1].DisabledAt == nil {
		t.Fatalf("expected disabled account state to be preserved, got %#v", all[1])
	}
}
