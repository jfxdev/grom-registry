package bunstore

import (
	"context"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func TestUpsertDiscoveredRepositoryReconcilesKnownRepositories(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)

		discovered := &registrydomain.Repository{
			ID: foundation.NewID(), ProjectID: projectID, Name: "api",
			Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationReconciled,
			Profile: constants.RepositoryProfileUnknown, ProfileSource: constants.ProfileSourceNone,
			ProfileConfidence: constants.ClassificationConfidenceNone, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		}
		if err := store.UpsertDiscoveredRepository(ctx, discovered); err != nil {
			t.Fatalf("reconcile known repository: %v", err)
		}
		repository, err := store.FindRepository(ctx, projectID, "api")
		if err != nil {
			t.Fatalf("find reconciled repository: %v", err)
		}
		if repository.ID != repositoryID || repository.Status != constants.RepositoryStatusActive || repository.CreationSource != constants.RepositoryCreationManual {
			t.Fatalf("known repository changed unexpectedly: %#v", repository)
		}

		if err := store.SetRepositoryStatus(ctx, repositoryID, constants.RepositoryStatusArchived); err != nil {
			t.Fatalf("archive repository: %v", err)
		}
		if err := store.UpsertDiscoveredRepository(ctx, discovered); err != nil {
			t.Fatalf("reconcile archived repository: %v", err)
		}
		repository, err = store.FindRepository(ctx, projectID, "api")
		if err != nil || repository.Status != constants.RepositoryStatusArchived {
			t.Fatalf("archived repository was reactivated: %#v, %v", repository, err)
		}

		discovered.Name = "worker"
		if err := store.UpsertDiscoveredRepository(ctx, discovered); err != nil {
			t.Fatalf("reconcile new repository: %v", err)
		}
		repository, err = store.FindRepository(ctx, projectID, "worker")
		if err != nil || repository.Status != constants.RepositoryStatusActive || repository.CreationSource != constants.RepositoryCreationReconciled {
			t.Fatalf("new discovered repository: %#v, %v", repository, err)
		}
	})
}
