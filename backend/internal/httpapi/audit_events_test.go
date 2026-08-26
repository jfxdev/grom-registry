package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	auditstore "github.com/jfxdev/grom/backend/internal/audit/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	identitystore "github.com/jfxdev/grom/backend/internal/identity/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/platform/database"
)

func newAuditEventsTestServer(t *testing.T) (*Server, *auditapp.Service, *identitydomain.User, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:httpapi-audit-events-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	identity := identityapp.New(identitystore.New(db), time.Hour)
	if err := identity.BootstrapAdmin(ctx, "admin@example.com", "admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := identity.Login(ctx, "admin@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	auditService := auditapp.New(auditstore.New(db))
	server := &Server{
		identity: identity,
		audit:    auditService,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return server, auditService, admin, ctx
}

func TestListAuditEventsRequiresAdmin(t *testing.T) {
	server, auditService, admin, ctx := newAuditEventsTestServer(t)
	if err := auditService.Record(ctx, foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID},
		constants.AuditProjectCreated, constants.AuditResourceProject, "proj-1", map[string]any{"reason": "x"}); err != nil {
		t.Fatal(err)
	}

	// A non-administrator is denied, and the denial discloses no event data.
	viewer := &identitydomain.User{ID: "viewer", SystemViewer: true}
	deniedResponse := httptest.NewRecorder()
	server.listAuditEvents(deniedResponse, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/audit-events", nil), viewer))
	if deniedResponse.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", deniedResponse.Code)
	}
	if strings.Contains(deniedResponse.Body.String(), constants.AuditProjectCreated) || strings.Contains(deniedResponse.Body.String(), "proj-1") {
		t.Fatalf("denial leaked audit event data: %s", deniedResponse.Body.String())
	}
}

func TestListAuditEventsReturnsFilteredPage(t *testing.T) {
	server, auditService, admin, ctx := newAuditEventsTestServer(t)
	actor := foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID}
	if err := auditService.Record(ctx, actor, constants.AuditProjectCreated, constants.AuditResourceProject, "proj-1", map[string]any{"reason": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := auditService.Record(ctx, actor, constants.AuditUserCreated, constants.AuditResourceUser, "user-1", nil); err != nil {
		t.Fatal(err)
	}

	adminResponse := httptest.NewRecorder()
	server.listAuditEvents(adminResponse, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/audit-events", nil), admin))
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", adminResponse.Code, adminResponse.Body.String())
	}
	var page struct {
		Items []struct {
			Action        string `json:"action"`
			ActorUsername string `json:"actorUsername"`
		} `json:"items"`
	}
	if err := json.NewDecoder(adminResponse.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 events, got %d", len(page.Items))
	}
	if page.Items[0].ActorUsername != "admin" {
		t.Fatalf("expected actor username resolved to admin, got %q", page.Items[0].ActorUsername)
	}

	// Action filter narrows the result set.
	filteredResponse := httptest.NewRecorder()
	server.listAuditEvents(filteredResponse, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/audit-events?action="+constants.AuditUserCreated, nil), admin))
	if filteredResponse.Code != http.StatusOK {
		t.Fatalf("expected 200 for filtered request, got %d", filteredResponse.Code)
	}
	if err := json.NewDecoder(filteredResponse.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Action != constants.AuditUserCreated {
		t.Fatalf("action filter did not narrow results: %+v", page.Items)
	}
}

func TestListAuditEventsRejectsInvalidQuery(t *testing.T) {
	server, _, admin, _ := newAuditEventsTestServer(t)
	for _, target := range []string{
		"http://grom/api/v1/audit-events?unknown=1",
		"http://grom/api/v1/audit-events?from=not-a-timestamp",
		"http://grom/api/v1/audit-events?to=not-a-timestamp",
		"http://grom/api/v1/audit-events?limit=0",
	} {
		response := httptest.NewRecorder()
		server.listAuditEvents(response, withUser(httptest.NewRequest(http.MethodGet, target, nil), admin))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", target, response.Code)
		}
	}
}
