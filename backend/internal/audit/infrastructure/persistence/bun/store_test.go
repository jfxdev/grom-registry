package bunstore

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
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

func newListTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:audit-list-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, username, password_hash, created_at) VALUES ('user-1', 'alex@example.com', 'alex', 'x', ?)`,
		time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO service_accounts (id, name, username, created_at) VALUES ('svc-1', 'CI Robot', 'ci-robot', ?)`,
		time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return New(db), ctx
}

func seedEvent(t *testing.T, store *Store, ctx context.Context, id, actorKind, actorID, action, resourceKind string, at time.Time) {
	t.Helper()
	if err := store.Record(ctx, &auditdomain.Event{
		ID: foundation.ID(id), ActorKind: actorKind, ActorID: foundation.ID(actorID),
		Action: action, ResourceKind: resourceKind, ResourceID: foundation.ID("res-" + id),
		MetadataJSON: `{"reason":"test"}`, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestListResolvesActorNames(t *testing.T) {
	store, ctx := newListTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	seedEvent(t, store, ctx, "e-user", "user", "user-1", "identity.login_succeeded", "authentication", base.Add(3*time.Second))
	seedEvent(t, store, ctx, "e-svc", "service_account", "svc-1", "identity.access_key_created", "service_account", base.Add(2*time.Second))
	seedEvent(t, store, ctx, "e-anon", "user", "", "identity.login_failed", "authentication", base.Add(1*time.Second))

	page, err := store.List(ctx, auditdomain.Filter{}, foundation.PageRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 events, got %d", len(page.Items))
	}
	// Newest first.
	if page.Items[0].ID != "e-user" || page.Items[2].ID != "e-anon" {
		t.Fatalf("unexpected ordering: %s..%s", page.Items[0].ID, page.Items[2].ID)
	}
	byID := map[foundation.ID]auditdomain.ListItem{}
	for _, item := range page.Items {
		byID[item.ID] = item
	}
	if got := byID["e-user"]; got.ActorName != "alex" || got.ActorUsername != "alex" {
		t.Fatalf("user actor not resolved: %+v", got)
	}
	if got := byID["e-svc"]; got.ActorName != "CI Robot" || got.ActorUsername != "ci-robot" {
		t.Fatalf("service-account actor not resolved: %+v", got)
	}
	if got := byID["e-anon"]; got.ActorName != "" || got.ActorUsername != "" {
		t.Fatalf("anonymous actor should not resolve: %+v", got)
	}
}

func TestListFilters(t *testing.T) {
	store, ctx := newListTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	seedEvent(t, store, ctx, "e1", "user", "user-1", "identity.login_succeeded", "authentication", base.Add(4*time.Second))
	seedEvent(t, store, ctx, "e2", "service_account", "svc-1", "identity.access_key_created", "service_account", base.Add(3*time.Second))
	seedEvent(t, store, ctx, "e3", "user", "user-1", "projects.project_created", "project", base.Add(2*time.Second))

	cases := []struct {
		name   string
		filter auditdomain.Filter
		want   []foundation.ID
	}{
		{"action", auditdomain.Filter{Action: "projects.project_created"}, []foundation.ID{"e3"}},
		{"resource", auditdomain.Filter{ResourceKind: "authentication"}, []foundation.ID{"e1"}},
		{"actor by username", auditdomain.Filter{Actor: "ci-rob"}, []foundation.ID{"e2"}},
		{"actor by name", auditdomain.Filter{Actor: "robot"}, []foundation.ID{"e2"}},
		{"actor by user", auditdomain.Filter{Actor: "alex"}, []foundation.ID{"e1", "e3"}},
		{"from bound", auditdomain.Filter{From: base.Add(3 * time.Second)}, []foundation.ID{"e1", "e2"}},
		{"to bound", auditdomain.Filter{To: base.Add(3 * time.Second)}, []foundation.ID{"e3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := store.List(ctx, tc.filter, foundation.PageRequest{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]foundation.ID, len(page.Items))
			for i, item := range page.Items {
				got[i] = item.ID
			}
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for _, id := range tc.want {
				found := false
				for _, g := range got {
					if g == id {
						found = true
					}
				}
				if !found {
					t.Fatalf("want %v, got %v", tc.want, got)
				}
			}
		})
	}
}

func TestListActorFilterTreatsLikeMetacharactersLiterally(t *testing.T) {
	store, ctx := newListTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	const literalActor = `literal%_\\actor`
	if _, err := store.db.ExecContext(ctx, `UPDATE users SET username = ? WHERE id = 'user-1'`, literalActor); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO users (id, email, username, password_hash, created_at) VALUES ('user-2', 'nearby@example.com', 'literalXYactor', 'x', ?)`,
		base); err != nil {
		t.Fatal(err)
	}
	seedEvent(t, store, ctx, "e-literal", "user", "user-1", "identity.login_succeeded", "authentication", base.Add(time.Second))
	seedEvent(t, store, ctx, "e-nearby", "user", "user-2", "identity.login_succeeded", "authentication", base)

	page, err := store.List(ctx, auditdomain.Filter{Actor: literalActor}, foundation.PageRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "e-literal" {
		t.Fatalf("expected only literal actor match, got %+v", page.Items)
	}
}

func TestListPaginatesWithKeyset(t *testing.T) {
	store, ctx := newListTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		seedEvent(t, store, ctx, "e"+string(rune('a'+i)), "user", "user-1", "identity.login_succeeded", "authentication", base.Add(time.Duration(i)*time.Second))
	}
	scope := "audit-events:test"
	first, err := store.List(ctx, auditdomain.Filter{}, foundation.PageRequest{Limit: 2, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("expected a full first page with cursor, got %d items cursor=%q", len(first.Items), first.NextCursor)
	}
	second, err := store.List(ctx, auditdomain.Filter{}, foundation.PageRequest{Limit: 2, Scope: scope, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 {
		t.Fatalf("expected 2 items on second page, got %d", len(second.Items))
	}
	// Pages must not overlap.
	seen := map[foundation.ID]bool{}
	for _, item := range append(append([]auditdomain.ListItem{}, first.Items...), second.Items...) {
		if seen[item.ID] {
			t.Fatalf("event %s appeared on multiple pages", item.ID)
		}
		seen[item.ID] = true
	}
	// Invalid cursor is rejected.
	if _, err := store.List(ctx, auditdomain.Filter{}, foundation.PageRequest{Limit: 2, Scope: scope, Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("expected invalid cursor to error")
	}
}

// TestListExposesSanitizedMetadata proves the sanitization guarantee holds on
// read: an event recorded through the application service (which redacts
// sensitive keys on write) exposes no plaintext credentials when listed.
func TestListExposesSanitizedMetadata(t *testing.T) {
	store, ctx := newListTestStore(t)
	service := auditapp.New(store)
	if err := service.Record(ctx, foundation.PrincipalRef{Kind: "user", ID: "user-1"},
		"identity.access_key_created", "service_account", "svc-1", map[string]any{
			"password":  "plaintext-password-for-test",
			"accessKey": "access-key-secret-for-test",
			"token":     "session-token-for-test",
			"reason":    "operator-request",
		}); err != nil {
		t.Fatal(err)
	}
	page, err := store.List(ctx, auditdomain.Filter{}, foundation.PageRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(page.Items))
	}
	metadata := string(page.Items[0].Metadata)
	for _, secret := range []string{"plaintext-password-for-test", "access-key-secret-for-test", "session-token-for-test"} {
		if strings.Contains(metadata, secret) {
			t.Fatalf("listed metadata leaked a credential: %s", metadata)
		}
	}
	if !strings.Contains(metadata, "operator-request") {
		t.Fatalf("listed metadata dropped a safe value: %s", metadata)
	}
}
