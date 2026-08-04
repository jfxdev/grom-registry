package database

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
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

func TestSQLiteMigrationLockPathSkipsMemoryDSNsAfterFilePrefixNormalization(t *testing.T) {
	for _, databaseURL := range []string{
		"sqlite://:memory:",
		"sqlite://file::memory:",
		"sqlite://file:migration-failure-test?mode=memory&cache=shared",
	} {
		if path := sqliteMigrationLockPath(databaseURL); path != "" {
			t.Fatalf("%s: expected no lock path, got %q", databaseURL, path)
		}
	}
	if path := sqliteMigrationLockPath("sqlite:///var/lib/grom/grom.db"); path != "/var/lib/grom/grom.db.migration.lock" {
		t.Fatalf("unexpected file-backed lock path %q", path)
	}
}

func TestSQLiteFileMigrationLockSerializesAndTimesOut(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "grom.db.migration.lock")
	unlock, err := sqliteFileMigrationLock(context.Background(), lockPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	_, err = sqliteFileMigrationLock(context.Background(), lockPath, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "migration lock timeout") {
		t.Fatalf("expected lock timeout, got %v", err)
	}

	unlock()
	unlock = nil
	secondUnlock, err := sqliteFileMigrationLock(context.Background(), lockPath, time.Second)
	if err != nil {
		t.Fatalf("expected lock acquisition after release: %v", err)
	}
	secondUnlock()
}
