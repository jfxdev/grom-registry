package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type unarchiveRepositoryStore struct {
	registrydomain.Store
	repository *registrydomain.Repository
	status     string
}

func (s *unarchiveRepositoryStore) FindRepositoryByID(context.Context, foundation.ID) (*registrydomain.Repository, error) {
	if s.repository == nil {
		return nil, sql.ErrNoRows
	}
	return s.repository, nil
}

func (s *unarchiveRepositoryStore) SetRepositoryStatus(_ context.Context, _ foundation.ID, status string) error {
	s.status = status
	if s.repository != nil {
		s.repository.Status = status
	}
	return nil
}

const unarchiveTestProjectID = foundation.ID("project-1")

func newUnarchiveTestServer(store *unarchiveRepositoryStore) *Server {
	project := &projectdomain.Project{ID: unarchiveTestProjectID, Slug: "payments"}
	return &Server{
		projects:     projectapp.New(&serverTestProjectRepository{project: project}),
		repositories: registryapp.NewRepositoryService(store),
	}
}

func unarchiveRequest() *http.Request {
	admin := &identitydomain.User{ID: foundation.NewID(), SystemAdmin: true}
	return withUserAndParams(
		httptest.NewRequest(http.MethodDelete, "http://grom/api/v1/projects/payments/repositories/repository-1/archive", nil),
		admin,
		map[string]string{"project": "payments", "repositoryId": "repository-1"},
	)
}

func TestUnarchiveRepositoryReenablesPushes(t *testing.T) {
	store := &unarchiveRepositoryStore{repository: &registrydomain.Repository{
		ID: foundation.ID("repository-1"), ProjectID: unarchiveTestProjectID, Name: "api", Status: constants.RepositoryStatusArchived,
	}}
	server := newUnarchiveTestServer(store)

	response := httptest.NewRecorder()
	server.unarchiveRepository(response, unarchiveRequest())

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", response.Code, response.Body.String())
	}
	if store.status != constants.RepositoryStatusEmpty {
		t.Fatalf("status after unarchive = %q", store.status)
	}
}

func TestUnarchiveRepositoryReturnsNotFoundForMissingRecord(t *testing.T) {
	server := newUnarchiveTestServer(&unarchiveRepositoryStore{})

	response := httptest.NewRecorder()
	server.unarchiveRepository(response, unarchiveRequest())

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestListMembershipsRejectsUnknownQueryParameter(t *testing.T) {
	server := &Server{}
	admin := &identitydomain.User{ID: foundation.NewID(), SystemAdmin: true}
	request := withUserAndParams(
		httptest.NewRequest(http.MethodGet, "http://grom/api/v1/projects/payments/members?unexpected=value", nil),
		admin,
		map[string]string{"project": "payments"},
	)

	response := httptest.NewRecorder()
	server.listMemberships(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}
