package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jfxdev/grom/backend/internal/foundation"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type searchTestStore struct {
	registrydomain.Store
	repository         *registrydomain.Repository
	findErr            error
	searchResults      foundation.PageResult[registrydomain.RepositorySearchResult]
	searchErr          error
	tagResults         foundation.PageResult[string]
	tagErr             error
	inventoryResults   foundation.PageResult[registrydomain.ManifestInventory]
	inventoryErr       error
	lastTagQuery       string
	lastInventoryQuery string
}

func (s *searchTestStore) FindRepository(context.Context, foundation.ID, string) (*registrydomain.Repository, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.repository, nil
}

func (s *searchTestStore) SearchRepositoriesAcrossProjects(context.Context, string, foundation.PageRequest) (foundation.PageResult[registrydomain.RepositorySearchResult], error) {
	return s.searchResults, s.searchErr
}

func (s *searchTestStore) SearchTagNamesPage(_ context.Context, _ foundation.ID, query string, _ foundation.PageRequest) (foundation.PageResult[string], error) {
	s.lastTagQuery = query
	return s.tagResults, s.tagErr
}

func (s *searchTestStore) ListManifestInventoryPage(_ context.Context, _ foundation.ID, query string, _ foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	s.lastInventoryQuery = query
	return s.inventoryResults, s.inventoryErr
}

func newSearchTestServer(store *searchTestStore) *Server {
	project := &projectdomain.Project{ID: unarchiveTestProjectID, Slug: "payments"}
	return &Server{
		projects:     projectapp.New(&serverTestProjectRepository{project: project}),
		repositories: registryapp.NewRepositoryService(store),
		inventory:    registryapp.NewInventoryService(store),
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func adminRequest(method, target string, params map[string]string) *http.Request {
	admin := &identitydomain.User{ID: foundation.NewID(), SystemAdmin: true}
	return withUserAndParams(httptest.NewRequest(method, target, nil), admin, params)
}

func TestSearchRepositoriesReturnsMatches(t *testing.T) {
	store := &searchTestStore{searchResults: foundation.PageResult[registrydomain.RepositorySearchResult]{
		Items: []registrydomain.RepositorySearchResult{{ID: foundation.ID("repository-1"), Name: "api", ProjectSlug: "payments"}},
	}}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.searchRepositories(response, adminRequest(http.MethodGet, "http://grom/api/v1/repositories?q=api", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page foundation.PageResult[registrydomain.RepositorySearchResult]
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "api" {
		t.Fatalf("unexpected search response: %#v", page)
	}
}

func TestSearchRepositoriesReportsStoreFailure(t *testing.T) {
	store := &searchTestStore{searchErr: errors.New("boom")}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.searchRepositories(response, adminRequest(http.MethodGet, "http://grom/api/v1/repositories?q=api", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", response.Code, response.Body.String())
	}
}

func TestSearchRepositoriesRejectsInvalidCursor(t *testing.T) {
	server := newSearchTestServer(&searchTestStore{})

	response := httptest.NewRecorder()
	server.searchRepositories(response, adminRequest(http.MethodGet, "http://grom/api/v1/repositories?q=api&cursor=not-valid-base64!!", nil))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestListTagsSearchRejectsInvalidCursor(t *testing.T) {
	store := &searchTestStore{repository: &registrydomain.Repository{ID: foundation.ID("repository-1"), ProjectID: unarchiveTestProjectID, Name: "api"}}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.listTags(response, adminRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repository-tags?repository=api&q=v1&cursor=not-valid-base64!!", map[string]string{"project": "payments"}))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestListTagsSearchesInventorySnapshotWhenQueryProvided(t *testing.T) {
	store := &searchTestStore{
		repository: &registrydomain.Repository{ID: foundation.ID("repository-1"), ProjectID: unarchiveTestProjectID, Name: "api"},
		tagResults: foundation.PageResult[string]{Items: []string{"v1.0.0"}},
	}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.listTags(response, adminRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repository-tags?repository=api&q=v1", map[string]string{"project": "payments"}))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if store.lastTagQuery != "v1" {
		t.Fatalf("expected search query %q, got %q", "v1", store.lastTagQuery)
	}
	var page foundation.PageResult[string]
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0] != "v1.0.0" {
		t.Fatalf("unexpected tag search response: %#v", page)
	}
}

func TestListTagsSearchReturnsEmptyForUnreconciledRepository(t *testing.T) {
	store := &searchTestStore{findErr: errors.New("not found")}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.listTags(response, adminRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repository-tags?repository=api&q=v1", map[string]string{"project": "payments"}))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page foundation.PageResult[string]
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected no tags for an unreconciled repository, got %#v", page)
	}
}

func TestListTagsSearchReportsStoreFailure(t *testing.T) {
	store := &searchTestStore{
		repository: &registrydomain.Repository{ID: foundation.ID("repository-1"), ProjectID: unarchiveTestProjectID, Name: "api"},
		tagErr:     errors.New("boom"),
	}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.listTags(response, adminRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repository-tags?repository=api&q=v1", map[string]string{"project": "payments"}))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", response.Code, response.Body.String())
	}
}

func TestListRepositoryInventorySearchesSnapshotWhenQueryProvided(t *testing.T) {
	store := &searchTestStore{
		repository:       &registrydomain.Repository{ID: foundation.ID("repository-1"), ProjectID: unarchiveTestProjectID, Name: "api"},
		inventoryResults: foundation.PageResult[registrydomain.ManifestInventory]{Items: []registrydomain.ManifestInventory{{Digest: "sha256:aaaa"}}},
	}
	server := newSearchTestServer(store)

	response := httptest.NewRecorder()
	server.listRepositoryInventory(response, adminRequest(http.MethodGet, "http://grom/api/v1/projects/payments/repository-inventory?repository=api&q=aaaa", map[string]string{"project": "payments"}))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if store.lastInventoryQuery != "aaaa" {
		t.Fatalf("expected search query %q, got %q", "aaaa", store.lastInventoryQuery)
	}
	var page foundation.PageResult[registrydomain.ManifestInventory]
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Digest != "sha256:aaaa" {
		t.Fatalf("unexpected inventory search response: %#v", page)
	}
}
