package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestPageResultAdvancesAndRejectsCrossScopeCursor(t *testing.T) {
	values := []string{"one", "two", "three"}
	firstRequest := httptest.NewRequest("GET", "/?limit=2", nil)
	first, err := pageResult(firstRequest, "users", values)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	secondRequest := httptest.NewRequest("GET", "/?cursor="+first.NextCursor, nil)
	second, err := pageResult(secondRequest, "users", values)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0] != "three" || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}
	wrongScopeRequest := httptest.NewRequest("GET", "/?cursor="+first.NextCursor, nil)
	if _, err := pageResult(wrongScopeRequest, "projects", values); err == nil {
		t.Fatal("expected cross-scope cursor to be rejected")
	}
}

func TestPageRequestRejectsInvalidLimit(t *testing.T) {
	for _, raw := range []string{"0", "101", "nope"} {
		t.Run(raw, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/?limit="+raw, nil)
			if _, _, err := pageRequest(request, "users"); err == nil {
				t.Fatalf("limit %q was accepted", raw)
			}
		})
	}
}

func TestPageResultReportsTotalPagesAndRejectsOutOfRangeCursor(t *testing.T) {
	values := []string{"one", "two", "three", "four", "five"}
	first, err := pageResult(httptest.NewRequest("GET", "/?limit=2", nil), "users", values)
	if err != nil {
		t.Fatal(err)
	}
	if first.PageCount != 3 {
		t.Fatalf("page count = %d, want 3", first.PageCount)
	}
	if _, err := pageResult(httptest.NewRequest("GET", "/?cursor=eyJ2IjoxLCJzIjoidXNlcnMiLCJvIjo2fQ", nil), "users", values); err == nil {
		t.Fatal("expected out-of-range cursor to be rejected")
	}
}
