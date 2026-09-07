package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	identitystore "github.com/jfxdev/grom/backend/internal/identity/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectstore "github.com/jfxdev/grom/backend/internal/projects/infrastructure/persistence/bun"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	registrystore "github.com/jfxdev/grom/backend/internal/registry/infrastructure/persistence/bun"
)

func newRegistryFlowTestServer(t *testing.T) (*Server, *identitydomain.User) {
	t.Helper()
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:httpapi-registry-flow-"+foundation.NewID().String()+"?mode=memory&cache=shared")
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
	store := registrystore.New(db)
	repositories := registryapp.NewRepositoryService(store)
	inventory := registryapp.NewInventoryService(store)
	audit := auditapp.New(&serverTestAuditStore{})
	server := &Server{
		identity: identity, audit: audit,
		projects:          projectapp.New(projectstore.New(db)),
		repositories:      repositories,
		inventory:         inventory,
		artifactDeletions: registryapp.NewArtifactDeletionService(store, repositories, inventory, nil, audit),
		lifecycle:         registryapp.NewLifecycleService(store, inventory, nil, audit),
		publicURL:         &url.URL{Scheme: "https", Host: "grom.example"},
		logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return server, admin
}

func TestListRegistryPolicyPresets(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()

	server.listRegistryPolicyPresets(response, httptest.NewRequest(http.MethodGet, "http://grom/api/v1/registry-policy-presets", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var presets []registrydomain.PolicyPreset
	if err := json.NewDecoder(response.Body).Decode(&presets); err != nil || len(presets) == 0 {
		t.Fatalf("expected at least one policy preset, got %#v, %v", presets, err)
	}
}

func TestRepositoryCRUDHandlers(t *testing.T) {
	server, admin := newRegistryFlowTestServer(t)

	createProjectResponse := httptest.NewRecorder()
	server.createProject(createProjectResponse, withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects", strings.NewReader(`{"name":"Payments","slug":"payments"}`)), admin))
	if createProjectResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createProjectResponse.Code, createProjectResponse.Body.String())
	}

	createRepoResponse := httptest.NewRecorder()
	server.createRepository(createRepoResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories", strings.NewReader(`{"name":"app","description":""}`)), admin, map[string]string{"project": "payments"}))
	if createRepoResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRepoResponse.Code, createRepoResponse.Body.String())
	}
	var repository registrydomain.Repository
	if err := json.NewDecoder(createRepoResponse.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}

	duplicateResponse := httptest.NewRecorder()
	server.createRepository(duplicateResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories", strings.NewReader(`{"name":"app","description":""}`)), admin, map[string]string{"project": "payments"}))
	if duplicateResponse.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a duplicate repository, got %d: %s", duplicateResponse.Code, duplicateResponse.Body.String())
	}

	invalidNameResponse := httptest.NewRecorder()
	server.createRepository(invalidNameResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories", strings.NewReader(`{"name":"UPPER CASE","description":""}`)), admin, map[string]string{"project": "payments"}))
	if invalidNameResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid repository name, got %d", invalidNameResponse.Code)
	}

	missingProjectResponse := httptest.NewRecorder()
	server.createRepository(missingProjectResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/missing/repositories", strings.NewReader(`{"name":"app"}`)), admin, map[string]string{"project": "missing"}))
	if missingProjectResponse.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing project, got %d", missingProjectResponse.Code)
	}

	getResponse := httptest.NewRecorder()
	server.getRepository(getResponse, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String(), nil), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getResponse.Code, getResponse.Body.String())
	}

	getMissingResponse := httptest.NewRecorder()
	server.getRepository(getMissingResponse, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repositories/missing", nil), admin, map[string]string{"project": "payments", "repositoryId": "missing"}))
	if getMissingResponse.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing repository, got %d", getMissingResponse.Code)
	}

	policiesResponse := httptest.NewRecorder()
	server.getRepositoryPolicies(policiesResponse, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String()+"/policies", nil), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if policiesResponse.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", policiesResponse.Code, policiesResponse.Body.String())
	}

	replaceResponse := httptest.NewRecorder()
	server.replaceRepositoryPolicies(replaceResponse, withUserAndParams(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String()+"/policies", strings.NewReader(`{"expectedVersion":0,"policies":[]}`)), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if replaceResponse.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", replaceResponse.Code, replaceResponse.Body.String())
	}

	staleVersionResponse := httptest.NewRecorder()
	server.replaceRepositoryPolicies(staleVersionResponse, withUserAndParams(httptest.NewRequest(http.MethodPut, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String()+"/policies", strings.NewReader(`{"expectedVersion":0,"policies":[]}`)), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if staleVersionResponse.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a stale policy version, got %d: %s", staleVersionResponse.Code, staleVersionResponse.Body.String())
	}

	archiveResponse := httptest.NewRecorder()
	server.archiveRepository(archiveResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String()+"/archive", nil), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if archiveResponse.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", archiveResponse.Code, archiveResponse.Body.String())
	}

	archiveMissingResponse := httptest.NewRecorder()
	server.archiveRepository(archiveMissingResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories/missing/archive", nil), admin, map[string]string{"project": "payments", "repositoryId": "missing"}))
	if archiveMissingResponse.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for archiving a missing repository, got %d", archiveMissingResponse.Code)
	}

	unarchiveResponse := httptest.NewRecorder()
	server.unarchiveRepository(unarchiveResponse, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String()+"/archive", nil), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if unarchiveResponse.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", unarchiveResponse.Code, unarchiveResponse.Body.String())
	}

	forbidden := httptest.NewRecorder()
	server.createRepository(forbidden, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories", strings.NewReader(`{"name":"other"}`)), &identitydomain.User{ID: foundation.NewID()}, map[string]string{"project": "payments"}))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-manager, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
}

func TestRemoveRepositoryRequiresArchival(t *testing.T) {
	server, admin := newRegistryFlowTestServer(t)

	server.createProject(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects", strings.NewReader(`{"name":"Payments","slug":"payments"}`)), admin))
	createRepoResponse := httptest.NewRecorder()
	server.createRepository(createRepoResponse, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/repositories", strings.NewReader(`{"name":"app"}`)), admin, map[string]string{"project": "payments"}))
	var repository registrydomain.Repository
	if err := json.NewDecoder(createRepoResponse.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}

	notArchived := httptest.NewRecorder()
	server.removeRepository(notArchived, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/projects/payments/repositories/"+repository.ID.String(), nil), admin, map[string]string{"project": "payments", "repositoryId": repository.ID.String()}))
	if notArchived.Code != http.StatusConflict {
		t.Fatalf("expected 409 for removing a non-archived repository, got %d: %s", notArchived.Code, notArchived.Body.String())
	}

	missing := httptest.NewRecorder()
	server.removeRepository(missing, withUserAndParams(httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/projects/payments/repositories/missing", nil), admin, map[string]string{"project": "payments", "repositoryId": "missing"}))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removing a missing repository, got %d", missing.Code)
	}
}

func TestArtifactDeletionHandlerGuardsAndMissingRepository(t *testing.T) {
	server, admin := newRegistryFlowTestServer(t)
	server.createProject(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects", strings.NewReader(`{"name":"Payments","slug":"payments"}`)), admin))

	missingProject := httptest.NewRecorder()
	server.previewArtifactDeletion(missingProject, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/missing/artifact-deletions/preview", strings.NewReader(`{"repository":"app","reference":"v1"}`)), admin, map[string]string{"project": "missing"}))
	if missingProject.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing project, got %d", missingProject.Code)
	}

	forbidden := httptest.NewRecorder()
	server.previewArtifactDeletion(forbidden, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/artifact-deletions/preview", strings.NewReader(`{"repository":"app","reference":"v1"}`)), &identitydomain.User{ID: foundation.NewID()}, map[string]string{"project": "payments"}))
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-manager, got %d", forbidden.Code)
	}

	invalidInput := httptest.NewRecorder()
	server.previewArtifactDeletion(invalidInput, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/artifact-deletions/preview", strings.NewReader(`{"repository":"","reference":""}`)), admin, map[string]string{"project": "payments"}))
	if invalidInput.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing repository/reference, got %d", invalidInput.Code)
	}

	pathTraversal := httptest.NewRecorder()
	server.previewArtifactDeletion(pathTraversal, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/artifact-deletions/preview", strings.NewReader(`{"repository":"../etc","reference":"v1"}`)), admin, map[string]string{"project": "payments"}))
	if pathTraversal.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a path-traversal repository, got %d", pathTraversal.Code)
	}

	repositoryMissing := httptest.NewRecorder()
	server.previewArtifactDeletion(repositoryMissing, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/artifact-deletions/preview", strings.NewReader(`{"repository":"app","reference":"v1"}`)), admin, map[string]string{"project": "payments"}))
	if repositoryMissing.Code != http.StatusConflict {
		t.Fatalf("expected 409 for previewing deletion in a repository that doesn't exist, got %d: %s", repositoryMissing.Code, repositoryMissing.Body.String())
	}

	deleteRepositoryMissing := httptest.NewRecorder()
	server.deleteArtifact(deleteRepositoryMissing, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/artifact-deletions", strings.NewReader(`{"repository":"app","reference":"v1"}`)), admin, map[string]string{"project": "payments"}))
	if deleteRepositoryMissing.Code != http.StatusConflict {
		t.Fatalf("expected 409 for deleting from a repository that doesn't exist, got %d: %s", deleteRepositoryMissing.Code, deleteRepositoryMissing.Body.String())
	}

	listMissingProject := httptest.NewRecorder()
	server.listArtifactDeletions(listMissingProject, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/missing/artifact-deletions", nil), admin, map[string]string{"project": "missing"}))
	if listMissingProject.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing project, got %d", listMissingProject.Code)
	}

	listMissingRepository := httptest.NewRecorder()
	server.listArtifactDeletions(listMissingRepository, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/artifact-deletions", nil), admin, map[string]string{"project": "payments"}))
	if listMissingRepository.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without a repository query parameter, got %d", listMissingRepository.Code)
	}

	listUnknownRepository := httptest.NewRecorder()
	server.listArtifactDeletions(listUnknownRepository, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/artifact-deletions?repository=app", nil), admin, map[string]string{"project": "payments"}))
	if listUnknownRepository.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a repository that doesn't exist, got %d: %s", listUnknownRepository.Code, listUnknownRepository.Body.String())
	}
}

func TestLifecycleHandlerGuardsAndMissingRepository(t *testing.T) {
	server, admin := newRegistryFlowTestServer(t)
	server.createProject(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects", strings.NewReader(`{"name":"Payments","slug":"payments"}`)), admin))

	missingProject := httptest.NewRecorder()
	server.createLifecyclePreview(missingProject, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/missing/lifecycle-previews", strings.NewReader(`{"repository":"app"}`)), admin, map[string]string{"project": "missing"}))
	if missingProject.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing project, got %d", missingProject.Code)
	}

	invalidInput := httptest.NewRecorder()
	server.createLifecyclePreview(invalidInput, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/lifecycle-previews", strings.NewReader(`{"repository":""}`)), admin, map[string]string{"project": "payments"}))
	if invalidInput.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a blank repository, got %d", invalidInput.Code)
	}

	repositoryMissing := httptest.NewRecorder()
	server.createLifecyclePreview(repositoryMissing, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/lifecycle-previews", strings.NewReader(`{"repository":"app"}`)), admin, map[string]string{"project": "payments"}))
	if repositoryMissing.Code != http.StatusConflict {
		t.Fatalf("expected 409 for previewing lifecycle on a repository that doesn't exist, got %d: %s", repositoryMissing.Code, repositoryMissing.Body.String())
	}

	getPreviewMissing := httptest.NewRecorder()
	server.getLifecyclePreview(getPreviewMissing, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/lifecycle-previews/missing", nil), admin, map[string]string{"project": "payments", "previewId": "missing"}))
	if getPreviewMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing lifecycle preview, got %d", getPreviewMissing.Code)
	}

	invalidRun := httptest.NewRecorder()
	server.createLifecycleRun(invalidRun, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/lifecycle-runs", strings.NewReader(`{"previewId":""}`)), admin, map[string]string{"project": "payments"}))
	if invalidRun.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a blank preview ID, got %d", invalidRun.Code)
	}

	previewMissingForRun := httptest.NewRecorder()
	server.createLifecycleRun(previewMissingForRun, withUserAndParams(httptest.NewRequest(http.MethodPost, "http://grom/api/v1/projects/payments/lifecycle-runs", strings.NewReader(`{"previewId":"missing"}`)), admin, map[string]string{"project": "payments"}))
	if previewMissingForRun.Code != http.StatusConflict {
		t.Fatalf("expected 409 for running an unknown lifecycle preview, got %d: %s", previewMissingForRun.Code, previewMissingForRun.Body.String())
	}

	getRunMissing := httptest.NewRecorder()
	server.getLifecycleRun(getRunMissing, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/lifecycle-runs/missing", nil), admin, map[string]string{"project": "payments", "runId": "missing"}))
	if getRunMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing lifecycle run, got %d", getRunMissing.Code)
	}

	listRunsMissingRepository := httptest.NewRecorder()
	server.listLifecycleRuns(listRunsMissingRepository, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/lifecycle-runs", nil), admin, map[string]string{"project": "payments"}))
	if listRunsMissingRepository.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without a repository query parameter, got %d", listRunsMissingRepository.Code)
	}

	listRunsUnknownRepository := httptest.NewRecorder()
	server.listLifecycleRuns(listRunsUnknownRepository, withUserAndParams(httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/lifecycle-runs?repository=app", nil), admin, map[string]string{"project": "payments"}))
	if listRunsUnknownRepository.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a repository that doesn't exist, got %d: %s", listRunsUnknownRepository.Code, listRunsUnknownRepository.Body.String())
	}
}
