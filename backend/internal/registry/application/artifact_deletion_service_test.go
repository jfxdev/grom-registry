package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type deletionTestStore struct {
	registrydomain.Store
	repository  *registrydomain.Repository
	inventory   []registrydomain.ManifestInventory
	deletion    *registrydomain.ArtifactDeletion
	marked      string
	markErr     error
	completeErr error
}

func (s *deletionTestStore) FindRepository(
	_ context.Context,
	projectID foundation.ID,
	name string,
) (*registrydomain.Repository, error) {
	if s.repository.ProjectID != projectID || s.repository.Name != name {
		return nil, context.Canceled
	}
	return s.repository, nil
}

func (s *deletionTestStore) FindRepositoryByID(
	_ context.Context,
	repositoryID foundation.ID,
) (*registrydomain.Repository, error) {
	if s.repository.ID != repositoryID {
		return nil, context.Canceled
	}
	return s.repository, nil
}

func (s *deletionTestStore) UpsertManifestObservation(
	_ context.Context,
	repositoryID foundation.ID,
	observation registrydomain.ManifestObservation,
	observedAt time.Time,
) error {
	for i := range s.inventory {
		if s.inventory[i].Digest == observation.Digest {
			if observation.Tag != "" {
				s.inventory[i].Tags = []string{observation.Tag}
			}
			return nil
		}
	}
	s.inventory = append(s.inventory, registrydomain.ManifestInventory{
		ID: foundation.NewID(), RepositoryID: repositoryID, Digest: observation.Digest,
		MediaType: observation.MediaType, ArtifactType: observation.ArtifactType,
		SubjectDigest: observation.SubjectDigest, Tags: []string{observation.Tag},
		State: constants.InventoryStateActive, FirstSeenAt: observedAt, LastSeenAt: observedAt,
	})
	return nil
}

func (s *deletionTestStore) CompleteInventoryReconciliation(
	context.Context,
	foundation.ID,
	[]string,
	[]string,
	time.Time,
) error {
	return nil
}

func (s *deletionTestStore) ListManifestInventory(
	context.Context,
	foundation.ID,
) ([]registrydomain.ManifestInventory, error) {
	return s.inventory, nil
}

func (s *deletionTestStore) SaveRepositoryProfile(
	context.Context,
	*registrydomain.Repository,
) error {
	return nil
}

func (s *deletionTestStore) CreateArtifactDeletion(
	_ context.Context,
	deletion *registrydomain.ArtifactDeletion,
) error {
	copy := *deletion
	s.deletion = &copy
	return nil
}

func (s *deletionTestStore) CompleteArtifactDeletion(
	_ context.Context,
	_ foundation.ID,
	status, message string,
	completedAt time.Time,
) error {
	s.deletion.Status = status
	s.deletion.Message = message
	s.deletion.CompletedAt = &completedAt
	return s.completeErr
}

func (s *deletionTestStore) MarkManifestDeleted(
	_ context.Context,
	_ foundation.ID,
	digest string,
	_ time.Time,
) error {
	s.marked = digest
	return s.markErr
}

type deletionTestDistribution struct {
	digest    string
	referrers []ManifestDescriptor
	deleted   string
}

func (d *deletionTestDistribution) ListRepositoryTags(context.Context, string) ([]string, error) {
	return []string{"dev"}, nil
}

func (d *deletionTestDistribution) ResolveManifest(
	_ context.Context,
	_, reference string,
) (string, bool, error) {
	if reference == "dev" || reference == d.digest {
		return d.digest, true, nil
	}
	return "", false, nil
}

func (d *deletionTestDistribution) FetchManifest(
	_ context.Context,
	_, reference string,
) (*ManifestMetadata, error) {
	digest := d.digest
	if reference != "dev" && reference != d.digest {
		digest = reference
	}
	return &ManifestMetadata{
		Digest: digest, MediaType: "application/vnd.oci.image.manifest.v1+json",
	}, nil
}

func (d *deletionTestDistribution) ListReferrers(
	context.Context,
	string,
	string,
) ([]ManifestDescriptor, error) {
	return d.referrers, nil
}

func (d *deletionTestDistribution) DeleteManifest(
	_ context.Context,
	_ string,
	digest string,
) error {
	d.deleted = digest
	return nil
}

type deletionTestAudit struct {
	actions  []string
	metadata []map[string]any
}

func (a *deletionTestAudit) Record(
	_ context.Context,
	_ foundation.PrincipalRef,
	action, _ string,
	_ foundation.ID,
	metadata map[string]any,
) error {
	a.actions = append(a.actions, action)
	a.metadata = append(a.metadata, metadata)
	return nil
}

func TestArtifactDeletionPersistsAndMarksInventory(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{
		ID: foundation.NewID(), ProjectID: projectID, Name: "api",
		Policies: []registrydomain.Policy{{
			Type: constants.RepositoryPolicyManualDeletion, Enabled: true, RequireReason: true,
		}},
	}
	store := &deletionTestStore{repository: repository}
	distribution := &deletionTestDistribution{digest: "sha256:artifact"}
	inventory := NewInventoryService(store)
	inventory.SetDistribution(distribution)
	audit := &deletionTestAudit{}
	service := NewArtifactDeletionService(
		store, NewRepositoryService(store), inventory, distribution, audit,
	)

	result, err := service.Execute(
		context.Background(), projectID, "payments", "api", "dev", "cleanup",
		"sha256:artifact", []string{"dev"},
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != constants.ArtifactDeletionCompleted ||
		store.marked != "sha256:artifact" ||
		distribution.deleted != "sha256:artifact" {
		t.Fatalf("unexpected deletion result: %#v", result)
	}
	if len(audit.actions) != 2 ||
		audit.actions[0] != constants.AuditArtifactDeletionStarted ||
		audit.actions[1] != constants.AuditArtifactDeletionCompleted {
		t.Fatalf("unexpected audit actions: %#v", audit.actions)
	}
}

func TestArtifactDeletionPreviewBlocksOCIReferrers(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{
		ID: foundation.NewID(), ProjectID: projectID, Name: "api",
	}
	store := &deletionTestStore{repository: repository}
	distribution := &deletionTestDistribution{
		digest:    "sha256:artifact",
		referrers: []ManifestDescriptor{{Digest: "sha256:signature"}},
	}
	inventory := NewInventoryService(store)
	inventory.SetDistribution(distribution)
	service := NewArtifactDeletionService(
		store, NewRepositoryService(store), inventory, distribution, nil,
	)

	preview, err := service.Preview(
		context.Background(), projectID, "payments", "api", "dev", "", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.BlockedReasons) != 1 ||
		len(preview.RelatedArtifacts) != 1 ||
		preview.RelatedArtifacts[0] != "sha256:signature" {
		t.Fatalf("expected referrer protection, got %#v", preview)
	}
}

func TestArtifactDeletionAuditsPostDeletePersistenceFailures(t *testing.T) {
	testCases := []struct {
		name        string
		markErr     error
		completeErr error
	}{
		{name: "inventory update", markErr: errors.New("inventory unavailable")},
		{name: "deletion record", completeErr: errors.New("deletion record unavailable")},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectID := foundation.NewID()
			repository := &registrydomain.Repository{
				ID: foundation.NewID(), ProjectID: projectID, Name: "api",
				Policies: []registrydomain.Policy{{
					Type: constants.RepositoryPolicyManualDeletion, Enabled: true, RequireReason: true,
				}},
			}
			store := &deletionTestStore{
				repository: repository, markErr: testCase.markErr, completeErr: testCase.completeErr,
			}
			distribution := &deletionTestDistribution{digest: "sha256:artifact"}
			inventory := NewInventoryService(store)
			inventory.SetDistribution(distribution)
			audit := &deletionTestAudit{}
			service := NewArtifactDeletionService(
				store, NewRepositoryService(store), inventory, distribution, audit,
			)

			deletion, err := service.Execute(
				context.Background(), projectID, "payments", "api", "dev", "cleanup",
				"sha256:artifact", []string{"dev"},
				foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
			)
			if err == nil || deletion == nil || distribution.deleted != "sha256:artifact" {
				t.Fatalf("expected a post-delete persistence failure: deletion=%#v, err=%v", deletion, err)
			}
			if len(audit.actions) != 2 || audit.actions[1] != constants.AuditArtifactDeletionFailed {
				t.Fatalf("expected failed post-delete audit action: %#v", audit.actions)
			}
			failure := audit.metadata[1]
			if failure["repository"] != "api" || failure["digest"] != "sha256:artifact" ||
				failure["manifestDeleted"] != true || failure["error"] != err.Error() {
				t.Fatalf("unexpected post-delete audit metadata: %#v", failure)
			}
		})
	}
}
