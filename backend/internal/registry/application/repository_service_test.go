package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type deletionPolicyStore struct {
	registrydomain.Store
	repository *registrydomain.Repository
}

type repositoryLifecycleStore struct {
	registrydomain.Store
	repository *registrydomain.Repository
	live       bool
	deleted    bool
}

type repositoryListStore struct {
	registrydomain.Store
	repositories []registrydomain.Repository
	upserts      int
}

func (s *repositoryListStore) ListRepositories(context.Context, foundation.ID) ([]registrydomain.Repository, error) {
	return append([]registrydomain.Repository(nil), s.repositories...), nil
}

func (s *repositoryListStore) UpsertDiscoveredRepository(_ context.Context, repository *registrydomain.Repository) error {
	s.upserts++
	s.repositories = append(s.repositories, *repository)
	return nil
}

func (s *repositoryListStore) SetRepositoryStatus(_ context.Context, repositoryID foundation.ID, status string) error {
	for i := range s.repositories {
		if s.repositories[i].ID == repositoryID {
			s.repositories[i].Status = status
		}
	}
	return nil
}

func (s *repositoryLifecycleStore) FindRepositoryByID(context.Context, foundation.ID) (*registrydomain.Repository, error) {
	return s.repository, nil
}

func (s *repositoryLifecycleStore) SetRepositoryStatus(_ context.Context, _ foundation.ID, status string) error {
	s.repository.Status = status
	return nil
}

func (s *repositoryLifecycleStore) RepositoryHasLiveManifests(context.Context, foundation.ID) (bool, error) {
	return s.live, nil
}

func (s *repositoryLifecycleStore) DeleteRepository(context.Context, foundation.ID) error {
	s.deleted = true
	return nil
}

func (s *deletionPolicyStore) FindRepository(
	context.Context,
	foundation.ID,
	string,
) (*registrydomain.Repository, error) {
	return s.repository, nil
}

func TestEvaluateDeletionDeterminesReasonRequirementBeforeProtection(t *testing.T) {
	service := NewRepositoryService(&deletionPolicyStore{repository: &registrydomain.Repository{
		Policies: []registrydomain.Policy{{
			Type: constants.RepositoryPolicyTagProtection, Enabled: true,
			TagPatterns: []string{"prod"}, PreventDeletion: true,
		}, {
			Type: constants.RepositoryPolicyManualDeletion, Enabled: true, RequireReason: true,
		}},
	}})

	requiresReason, err := service.EvaluateDeletion(
		context.Background(), foundation.ID("project"), "api", []string{"prod"}, "", true,
	)
	if err == nil {
		t.Fatal("expected protected tag deletion to be rejected")
	}
	if !requiresReason {
		t.Fatal("expected reason requirement from a later policy to be preserved")
	}
}

func TestArchiveThenRemoveEmptyRepository(t *testing.T) {
	projectID := foundation.ID("project")
	repositoryID := foundation.ID("repository")
	store := &repositoryLifecycleStore{repository: &registrydomain.Repository{
		ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusEmpty,
	}}
	service := NewRepositoryService(store)
	actor := foundation.PrincipalRef{Kind: "user", ID: foundation.ID("admin")}
	if err := service.Archive(context.Background(), projectID, repositoryID, actor); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if store.repository.Status != constants.RepositoryStatusArchived {
		t.Fatalf("status = %q", store.repository.Status)
	}
	if err := service.Remove(context.Background(), projectID, repositoryID, actor); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !store.deleted {
		t.Fatal("expected repository record to be removed")
	}
}

func TestRemoveRepositoryRequiresArchiveAndEmptyInventory(t *testing.T) {
	projectID := foundation.ID("project")
	repositoryID := foundation.ID("repository")
	store := &repositoryLifecycleStore{repository: &registrydomain.Repository{
		ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusEmpty,
	}}
	service := NewRepositoryService(store)
	err := service.Remove(context.Background(), projectID, repositoryID, foundation.PrincipalRef{})
	if !errors.Is(err, ErrRepositoryNotArchived) {
		t.Fatalf("expected archive requirement, got %v", err)
	}
	store.repository.Status = constants.RepositoryStatusArchived
	store.live = true
	err = service.Remove(context.Background(), projectID, repositoryID, foundation.PrincipalRef{})
	if !errors.Is(err, ErrRepositoryNotEmpty) {
		t.Fatalf("expected empty inventory requirement, got %v", err)
	}
}

func TestListDoesNotUpsertRepositoriesAlreadyKnownToBeActive(t *testing.T) {
	projectID := foundation.ID("project")
	store := &repositoryListStore{repositories: []registrydomain.Repository{{
		ID: foundation.ID("repository"), ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusActive,
	}}}

	repositories, err := NewRepositoryService(store).List(context.Background(), projectID, []string{"api"}, true)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if store.upserts != 0 {
		t.Fatalf("upserts = %d, want 0", store.upserts)
	}
	if len(repositories) != 1 || repositories[0].Name != "api" {
		t.Fatalf("repositories = %#v", repositories)
	}
}

func TestListCreatesRepositoryDiscoveredFromCatalog(t *testing.T) {
	projectID := foundation.ID("project")
	store := &repositoryListStore{}

	repositories, err := NewRepositoryService(store).List(context.Background(), projectID, []string{"api"}, true)
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if store.upserts != 1 {
		t.Fatalf("upserts = %d, want 1", store.upserts)
	}
	if len(repositories) != 1 || repositories[0].Name != "api" || repositories[0].Status != constants.RepositoryStatusActive {
		t.Fatalf("repositories = %#v", repositories)
	}
}
