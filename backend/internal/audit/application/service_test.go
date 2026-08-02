package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	auditdomain "github.com/jfxdev/grom/backend/internal/audit/domain"
	"github.com/jfxdev/grom/backend/internal/foundation"
)

type testStore struct {
	event *auditdomain.Event
	err   error
}

func (s *testStore) Record(_ context.Context, event *auditdomain.Event) error {
	s.event = event
	return s.err
}

func (s *testStore) RecordOnce(_ context.Context, event *auditdomain.Event) error {
	s.event = event
	return s.err
}

func TestRecordBuildsSanitizedStructuredEvent(t *testing.T) {
	store := &testStore{}
	service := New(store)
	if err := service.Record(context.Background(), foundation.PrincipalRef{Kind: "user", ID: "actor"}, "test.action", "user", "target", map[string]any{
		"tokenId": "public-id-only",
		"reason":  "operator-request",
	}); err != nil {
		t.Fatal(err)
	}
	if store.event == nil || store.event.ID == "" || store.event.CreatedAt.IsZero() {
		t.Fatalf("event was not fully populated: %#v", store.event)
	}
	if strings.Contains(store.event.MetadataJSON, "secret") {
		t.Fatalf("event metadata contains a secret: %s", store.event.MetadataJSON)
	}
}

func TestRecordPropagatesMarshalAndStoreErrors(t *testing.T) {
	marshalStore := &testStore{}
	service := New(marshalStore)
	if err := service.Record(context.Background(), foundation.PrincipalRef{}, "test.action", "test", "id", map[string]any{"invalid": func() {}}); err == nil {
		t.Fatal("expected metadata marshal error")
	}

	storeErr := errors.New("store unavailable")
	service = New(&testStore{err: storeErr})
	if err := service.RecordOnce(context.Background(), foundation.ID("event"), foundation.PrincipalRef{}, "test.action", "test", "id", nil); !errors.Is(err, storeErr) {
		t.Fatalf("expected store error %v, got %v", storeErr, err)
	}
}
