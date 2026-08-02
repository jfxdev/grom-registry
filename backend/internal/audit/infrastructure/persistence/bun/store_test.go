package bunstore

import (
	"context"
	"log/slog"
	"testing"
	"time"

	auditdomain "github.com/jfxdev/grom/backend/internal/audit/domain"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/jfxdev/grom/backend/internal/platform/database"
)

func TestStorePersistsAndDeduplicatesEvents(t *testing.T) {
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:audit-store-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}

	store := New(db)
	event := &auditdomain.Event{
		ID: foundation.ID("event-1"), ActorKind: "user", ActorID: foundation.ID("actor"),
		Action: "test.action", ResourceKind: "user", ResourceID: foundation.ID("target"),
		MetadataJSON: `{"reason":"test"}`, CreatedAt: time.Now().UTC(),
	}
	if err := store.Record(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordOnce(ctx, event); err != nil {
		t.Fatal(err)
	}
	count, err := db.NewSelect().Table("audit_events").Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one deduplicated event, got %d", count)
	}
}
