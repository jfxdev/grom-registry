package domain

import (
	"context"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Store interface {
	CreateRepository(ctx context.Context, repository *Repository) error
	EnsureRepository(ctx context.Context, repository *Repository) (*Repository, bool, error)
	ListRepositories(ctx context.Context, projectID foundation.ID) ([]Repository, error)
	FindRepository(ctx context.Context, projectID foundation.ID, name string) (*Repository, error)
	FindRepositoryByID(ctx context.Context, repositoryID foundation.ID) (*Repository, error)
	RepositoryExists(ctx context.Context, projectID foundation.ID, name string) (bool, error)
	ProjectHasRepositories(ctx context.Context, projectID foundation.ID) (bool, error)
	UpsertDiscoveredRepository(ctx context.Context, repository *Repository) error
	SetRepositoryStatus(ctx context.Context, repositoryID foundation.ID, status string) error
	RepositoryHasLiveManifests(ctx context.Context, repositoryID foundation.ID) (bool, error)
	DeleteRepository(ctx context.Context, repositoryID foundation.ID) error
	SaveRepositoryProfile(ctx context.Context, repository *Repository) error
	ReplaceRepositoryPolicies(ctx context.Context, repositoryID foundation.ID, expectedVersion int, policies []Policy, updatedAt time.Time) (int, error)
	UpsertManifestObservation(ctx context.Context, repositoryID foundation.ID, observation ManifestObservation, observedAt time.Time) error
	CompleteInventoryReconciliation(ctx context.Context, repositoryID foundation.ID, seenTags, seenDigests []string, observedAt time.Time) error
	ListManifestInventory(ctx context.Context, repositoryID foundation.ID) ([]ManifestInventory, error)
	ListManifestInventoryPage(ctx context.Context, repositoryID foundation.ID, request foundation.PageRequest) (foundation.PageResult[ManifestInventory], error)
	MarkManifestDeleted(ctx context.Context, repositoryID foundation.ID, digest string, deletedAt time.Time) error
	CreateArtifactDeletion(ctx context.Context, deletion *ArtifactDeletion) error
	CompleteArtifactDeletion(ctx context.Context, deletionID foundation.ID, status, message string, completedAt time.Time) error
	ListArtifactDeletions(ctx context.Context, repositoryID foundation.ID) ([]ArtifactDeletion, error)
	ListArtifactDeletionsPage(ctx context.Context, repositoryID foundation.ID, request foundation.PageRequest) (foundation.PageResult[ArtifactDeletion], error)
	CreateLifecyclePreview(ctx context.Context, preview *LifecyclePreview) error
	FindLifecyclePreview(ctx context.Context, previewID foundation.ID) (*LifecyclePreview, error)
	SetLifecyclePreviewStatus(ctx context.Context, previewID foundation.ID, status string) error
	AcquireLifecycleLock(ctx context.Context, repositoryID, runID foundation.ID, now time.Time) error
	ReleaseLifecycleLock(ctx context.Context, repositoryID, runID foundation.ID) error
	CreateLifecycleRunFromPreview(ctx context.Context, run *LifecycleRun) error
	UpdateLifecycleRunItem(ctx context.Context, item *LifecycleRunItem) error
	CompleteLifecycleRun(ctx context.Context, runID foundation.ID, status string, completedAt time.Time) error
	ListLifecycleRuns(ctx context.Context, repositoryID foundation.ID) ([]LifecycleRun, error)
	ListLifecycleRunsPage(ctx context.Context, repositoryID foundation.ID, request foundation.PageRequest) (foundation.PageResult[LifecycleRun], error)
	FindLifecycleRun(ctx context.Context, runID foundation.ID) (*LifecycleRun, error)
}
