package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/jfxdev/grom/backend/internal/platform/diagnostics"
)

type diagnosticsDatabaseProbe struct{ state database.Diagnostics }

func (probe diagnosticsDatabaseProbe) Inspect(context.Context) database.Diagnostics {
	return probe.state
}

type diagnosticsDistributionProbe struct{ version string }

func (probe diagnosticsDistributionProbe) Probe(context.Context) (string, error) {
	return probe.version, nil
}

type diagnosticsSigningProbe struct{}

func (diagnosticsSigningProbe) Probe() (string, string, error) { return "RS256", "grom-default", nil }

func installationDiagnosticsService() *diagnostics.Service {
	appliedAt := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	return diagnostics.New(diagnostics.Options{
		DatabaseKind: "sqlite",
		Deployment:   diagnostics.Deployment{Profile: "strict"},
		Database: diagnosticsDatabaseProbe{state: database.Diagnostics{
			Kind: database.SQLite, Available: true,
			Migration: database.MigrationDiagnostics{Status: database.MigrationCurrent, AppliedVersion: "202608300001", AppliedAt: &appliedAt},
		}},
		Distribution: diagnosticsDistributionProbe{version: "registry/2.0"},
		Signing:      diagnosticsSigningProbe{},
	})
}

func TestInstallationDiagnosticsRequiresAdministratorAndNeverCaches(t *testing.T) {
	server, _, admin := newFlowTestServer(t)
	server.diagnostics = installationDiagnosticsService()

	unauthenticated := httptest.NewRecorder()
	server.requireSession(http.HandlerFunc(server.getInstallationDiagnostics)).ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "http://grom/api/v1/settings/diagnostics", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthenticated.Code)
	}

	forbidden := httptest.NewRecorder()
	server.getInstallationDiagnostics(forbidden, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/settings/diagnostics", nil), &identitydomain.User{}))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", forbidden.Code)
	}

	response := httptest.NewRecorder()
	server.getInstallationDiagnostics(response, withUser(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/settings/diagnostics", nil), admin))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("cache control = %q", cacheControl)
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "postgres://") {
		t.Fatalf("diagnostics exposed sensitive material: %s", response.Body.String())
	}
	var result diagnostics.Result
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Database.Status != "available" || result.Migration.Status != database.MigrationCurrent || result.Distribution.APIVersion == nil || *result.Distribution.APIVersion != "registry/2.0" || result.Signing.KeyID == nil || *result.Signing.KeyID != "grom-default" {
		t.Fatalf("unexpected diagnostics: %#v", result)
	}
}
