package application

import (
	"context"
	"encoding/json"
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
		"tokenId":    "public-id-only",
		"token_id":   "public-id-underscore",
		"token-id":   "public-id-hyphen",
		"password":   "plaintext-password-for-test",
		"TOKEN":      "standalone-token",
		"t_o_k_e_n":  "underscore-token",
		"t-o-k-e-n":  "hyphen-token",
		"ACCESS_KEY": "access-key-token",
		"access-key": "hyphen-access-key-token",
		"reason":     "operator-request",
		"nested":     map[string]any{"secret": "nested-secret-for-test"},
		"items":      []any{map[string]any{"password": "nested-password-for-test"}},
	}); err != nil {
		t.Fatal(err)
	}
	if store.event == nil || store.event.ID == "" || store.event.CreatedAt.IsZero() {
		t.Fatalf("event was not fully populated: %#v", store.event)
	}
	for _, credential := range []string{"plaintext-password-for-test", "nested-secret-for-test", "nested-password-for-test", "standalone-token", "underscore-token", "hyphen-token", "access-key-token", "hyphen-access-key-token"} {
		if strings.Contains(store.event.MetadataJSON, credential) {
			t.Fatalf("event metadata contains a plaintext credential: %s", store.event.MetadataJSON)
		}
	}
	for _, allowed := range []string{"public-id-only", "public-id-underscore", "public-id-hyphen"} {
		if !strings.Contains(store.event.MetadataJSON, allowed) {
			t.Fatalf("event metadata removed allowed token identifier: %s", store.event.MetadataJSON)
		}
	}
}

type typedMetadata struct {
	Password string `json:"password"`
	Nested   string `json:"nested"`
}

func TestRecordNormalizesTypedValuesAndRawJSONBeforeSanitizing(t *testing.T) {
	store := &testStore{}
	service := New(store)
	metadata := map[string]any{
		"typed": typedMetadata{Password: "typed-password", Nested: "kept"},
		"slice": []typedMetadata{{Password: "slice-password", Nested: "slice-kept"}},
		"raw":   json.RawMessage(`{"password":"raw-password","kept":"raw-kept"}`),
	}
	if err := service.Record(context.Background(), foundation.PrincipalRef{}, "test.action", "test", "id", metadata); err != nil {
		t.Fatal(err)
	}
	for _, credential := range []string{"typed-password", "slice-password", "raw-password"} {
		if strings.Contains(store.event.MetadataJSON, credential) {
			t.Fatalf("typed metadata contains a plaintext credential: %s", store.event.MetadataJSON)
		}
	}
	for _, value := range []string{"kept", "slice-kept", "raw-kept"} {
		if !strings.Contains(store.event.MetadataJSON, value) {
			t.Fatalf("typed metadata lost safe value %q: %s", value, store.event.MetadataJSON)
		}
	}
}

func TestRecordRejectsCyclicMetadata(t *testing.T) {
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	service := New(&testStore{})
	if err := service.Record(context.Background(), foundation.PrincipalRef{}, "test.action", "test", "id", cyclic); err == nil {
		t.Fatal("expected cyclic metadata to be rejected")
	}

	cyclicSlice := make([]any, 1)
	cyclicSlice[0] = cyclicSlice
	if err := service.Record(context.Background(), foundation.PrincipalRef{}, "test.action", "test", "id", map[string]any{"items": cyclicSlice}); err == nil {
		t.Fatal("expected cyclic slice metadata to be rejected")
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
