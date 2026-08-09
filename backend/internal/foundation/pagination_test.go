package foundation

import "testing"

func TestPageCursorRoundTripAndScope(t *testing.T) {
	raw, err := EncodePageCursor(PageCursor{Scope: "inventory:alpha/api", Timestamp: "2026-08-09T12:00:00Z", ID: "manifest-1"})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := DecodePageCursor(raw, "inventory:alpha/api")
	if err != nil || cursor.ID != "manifest-1" || cursor.Timestamp == "" {
		t.Fatalf("unexpected cursor: %#v, %v", cursor, err)
	}
	if _, err := DecodePageCursor(raw, "inventory:beta/api"); err == nil {
		t.Fatal("expected scope mismatch")
	}
	if _, err := DecodePageCursor("invalid", "inventory:alpha/api"); err == nil {
		t.Fatal("expected malformed cursor")
	}
}
