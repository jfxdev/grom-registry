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
	markedAll   []string
	markErr     error
	completeErr error
}

func (s *deletionTestStore) ListManifestInventoryPage(_ context.Context, _ foundation.ID, _ foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	return foundation.PageResult[registrydomain.ManifestInventory]{Items: s.inventory}, nil
}

func (s *deletionTestStore) ListArtifactDeletionsPage(_ context.Context, _ foundation.ID, _ foundation.PageRequest) (foundation.PageResult[registrydomain.ArtifactDeletion], error) {
	if s.deletion == nil {
		return foundation.PageResult[registrydomain.ArtifactDeletion]{}, nil
	}
	return foundation.PageResult[registrydomain.ArtifactDeletion]{Items: []registrydomain.ArtifactDeletion{*s.deletion}}, nil
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
			s.inventory[i].Platforms = observation.Platforms
			if observation.Tag != "" {
				s.inventory[i].Tags = []string{observation.Tag}
				s.inventory[i].State = constants.InventoryStateActive
			}
			return nil
		}
	}
	tags := []string{}
	state := constants.InventoryStateUntagged
	if observation.Tag != "" {
		tags = []string{observation.Tag}
		state = constants.InventoryStateActive
	}
	s.inventory = append(s.inventory, registrydomain.ManifestInventory{
		ID: foundation.NewID(), RepositoryID: repositoryID, Digest: observation.Digest,
		MediaType: observation.MediaType, ArtifactType: observation.ArtifactType,
		SubjectDigest: observation.SubjectDigest, Platforms: observation.Platforms, Tags: tags,
		State: state, FirstSeenAt: observedAt, LastSeenAt: observedAt,
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
	s.markedAll = append(s.markedAll, digest)
	return s.markErr
}

type deletionTestDistribution struct {
	digest          string
	children        []ManifestMetadata
	referrers       []ManifestDescriptor
	deleted         string
	deletedAll      []string
	deleteErr       error
	deleteErrDigest string
	missing         map[string]bool
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
	for _, child := range d.children {
		if reference == child.Digest {
			if d.missing[reference] {
				return "", false, nil
			}
			return child.Digest, true, nil
		}
	}
	return "", false, nil
}

func (d *deletionTestDistribution) FetchManifest(
	_ context.Context,
	_, reference string,
) (*ManifestMetadata, error) {
	for _, referrer := range d.referrers {
		if reference == referrer.Digest {
			return &ManifestMetadata{Digest: referrer.Digest, ManifestSize: referrer.Size}, nil
		}
	}
	if reference != "dev" && reference != d.digest {
		for _, child := range d.children {
			if reference == child.Digest {
				copy := child
				return &copy, nil
			}
		}
	}
	return &ManifestMetadata{
		Digest: d.digest, MediaType: "application/vnd.oci.image.manifest.v1+json",
		Platforms: func() []registrydomain.ManifestPlatform {
			platforms := make([]registrydomain.ManifestPlatform, 0, len(d.children))
			for _, child := range d.children {
				platforms = append(platforms, registrydomain.ManifestPlatform{Digest: child.Digest})
			}
			return platforms
		}(),
		Children: d.children,
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
	d.deletedAll = append(d.deletedAll, digest)
	if d.deleteErr != nil && (d.deleteErrDigest == "" || d.deleteErrDigest == digest) {
		return d.deleteErr
	}
	return nil
}

type deletionTestAudit struct {
	actions  []string
	metadata []map[string]any
	err      error
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
	return a.err
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
		"sha256:artifact", []string{"dev"}, []string{},
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

func TestArtifactDeletionIncludesOnlyThePreviewedOrphanedPlatformChildren(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "api"}
	store := &deletionTestStore{repository: repository}
	distribution := &deletionTestDistribution{
		digest: "sha256:index",
		children: []ManifestMetadata{{
			Digest: "sha256:platform", MediaType: "application/vnd.oci.image.manifest.v1+json",
		}},
	}
	inventory := NewInventoryService(store)
	inventory.SetDistribution(distribution)
	service := NewArtifactDeletionService(store, NewRepositoryService(store), inventory, distribution, nil)

	preview, err := service.Preview(context.Background(), projectID, "payments", "api", "dev", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ChildDigests) != 1 || preview.ChildDigests[0] != "sha256:platform" {
		t.Fatalf("unexpected child deletion preview: %#v", preview)
	}
	result, err := service.Execute(
		context.Background(), projectID, "payments", "api", "dev", "cleanup",
		"sha256:index", []string{"dev"}, preview.ChildDigests,
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != constants.ArtifactDeletionCompleted ||
		len(distribution.deletedAll) != 2 || distribution.deletedAll[0] != "sha256:index" || distribution.deletedAll[1] != "sha256:platform" ||
		len(store.markedAll) != 2 {
		t.Fatalf("unexpected index deletion: result=%#v deleted=%#v marked=%#v", result, distribution.deletedAll, store.markedAll)
	}
}

func TestArtifactDeletionRecordsSuccessfulDigestsBeforeFailedCompletion(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "api"}
	store := &deletionTestStore{repository: repository, completeErr: errors.New("completion unavailable")}
	distribution := &deletionTestDistribution{
		digest:    "sha256:index",
		children:  []ManifestMetadata{{Digest: "sha256:child"}},
		deleteErr: errors.New("child delete failed"), deleteErrDigest: "sha256:child",
	}
	inventory := NewInventoryService(store)
	inventory.SetDistribution(distribution)
	service := NewArtifactDeletionService(store, NewRepositoryService(store), inventory, distribution, nil)

	_, err := service.Execute(
		context.Background(), projectID, "payments", "api", "dev", "cleanup",
		"sha256:index", []string{"dev"}, []string{"sha256:child"},
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)
	if !errors.Is(err, store.completeErr) {
		t.Fatalf("expected completion error, got %v", err)
	}
	if len(store.markedAll) != 1 || store.markedAll[0] != "sha256:index" {
		t.Fatalf("successfully deleted target was not recorded before completion: %#v", store.markedAll)
	}
}

func TestArtifactDeletionCompletionCountsOnlyDeletedChildren(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "api"}
	store := &deletionTestStore{repository: repository}
	distribution := &deletionTestDistribution{
		digest:   "sha256:index",
		children: []ManifestMetadata{{Digest: "sha256:child"}},
		missing:  map[string]bool{"sha256:child": true},
	}
	inventory := NewInventoryService(store)
	inventory.SetDistribution(distribution)
	service := NewArtifactDeletionService(store, NewRepositoryService(store), inventory, distribution, nil)

	result, err := service.Execute(
		context.Background(), projectID, "payments", "api", "dev", "cleanup",
		"sha256:index", []string{"dev"}, []string{"sha256:child"},
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "manifest and 0 unreferenced child manifests deleted; storage is reclaimed by a later garbage collection" {
		t.Fatalf("unexpected completion message %q", result.Message)
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

func TestArtifactDeletionAuditsExecutionFailures(t *testing.T) {
	testCases := []struct {
		name            string
		deleteErr       error
		markErr         error
		completeErr     error
		manifestDeleted bool
	}{
		{name: "distribution deletion", deleteErr: errors.New("distribution unavailable")},
		{name: "inventory update", markErr: errors.New("inventory unavailable"), manifestDeleted: true},
		{name: "deletion record", completeErr: errors.New("deletion record unavailable"), manifestDeleted: true},
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
			distribution := &deletionTestDistribution{
				digest: "sha256:artifact", deleteErr: testCase.deleteErr,
			}
			inventory := NewInventoryService(store)
			inventory.SetDistribution(distribution)
			audit := &deletionTestAudit{}
			service := NewArtifactDeletionService(
				store, NewRepositoryService(store), inventory, distribution, audit,
			)

			deletion, err := service.Execute(
				context.Background(), projectID, "payments", "api", "dev", "cleanup",
				"sha256:artifact", []string{"dev"}, []string{},
				foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
			)
			if err == nil || deletion == nil || distribution.deleted != "sha256:artifact" {
				t.Fatalf("expected a post-delete persistence failure: deletion=%#v, err=%v", deletion, err)
			}
			if len(audit.actions) != 2 || audit.actions[1] != constants.AuditArtifactDeletionFailed {
				t.Fatalf("expected failed post-delete audit action: %#v", audit.actions)
			}
			failure := audit.metadata[1]
			affectedTags, tagsOK := failure["affectedTags"].([]string)
			if failure["repository"] != "api" || failure["digest"] != "sha256:artifact" ||
				!tagsOK || len(affectedTags) != 1 || affectedTags[0] != "dev" ||
				failure["reason"] != "cleanup" || failure["message"] != deletion.Message ||
				failure["manifestDeleted"] != testCase.manifestDeleted ||
				failure["error"] != err.Error() {
				t.Fatalf("unexpected deletion failure audit metadata: %#v", failure)
			}
		})
	}
}

func TestArtifactDeletionDoesNotReachDistributionWhenStartAuditFails(t *testing.T) {
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
	service := NewArtifactDeletionService(
		store, NewRepositoryService(store), inventory, distribution,
		&deletionTestAudit{err: errors.New("audit unavailable")},
	)

	deletion, err := service.Execute(
		context.Background(), projectID, "payments", "api", "dev", "cleanup",
		"sha256:artifact", []string{"dev"}, []string{},
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)

	if err == nil {
		t.Fatal("expected audit failure")
	}
	if deletion != nil || distribution.deleted != "" {
		t.Fatalf("manifest deletion proceeded despite audit failure: deletion=%#v, distribution=%q", deletion, distribution.deleted)
	}
	if store.deletion == nil || store.deletion.Status != constants.ArtifactDeletionFailed {
		t.Fatalf("expected persisted failed deletion operation, got %#v", store.deletion)
	}
}

func TestArtifactDeletionListPageAddsRepositoryName(t *testing.T) {
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: foundation.NewID(), Name: "api"}
	store := &deletionTestStore{repository: repository, deletion: &registrydomain.ArtifactDeletion{ID: foundation.NewID(), RepositoryID: repository.ID, Digest: "sha256:test"}}
	service := NewArtifactDeletionService(store, nil, nil, nil, nil)
	page, err := service.ListPage(context.Background(), repository.ProjectID, "api", foundation.PageRequest{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Repository != "api" {
		t.Fatalf("unexpected page: %#v", page)
	}
}
