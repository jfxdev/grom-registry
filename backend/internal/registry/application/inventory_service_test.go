package application

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type missingProbeDistribution struct {
	resolveCalls int
	fetchCalls   int
	available    bool
}

func (*missingProbeDistribution) ListRepositoryTags(context.Context, string) ([]string, error) {
	return []string{}, nil
}

func (d *missingProbeDistribution) ResolveManifest(_ context.Context, _, reference string) (string, bool, error) {
	d.resolveCalls++
	return reference, d.available, nil
}

func (d *missingProbeDistribution) FetchManifest(_ context.Context, _, reference string) (*ManifestMetadata, error) {
	d.fetchCalls++
	return &ManifestMetadata{Digest: reference}, nil
}

func (*missingProbeDistribution) ListReferrers(context.Context, string, string) ([]ManifestDescriptor, error) {
	return nil, nil
}

func (*missingProbeDistribution) DeleteManifest(context.Context, string, string) error { return nil }

func TestReconcileBacksOffMissingManifestProbesAndRetriesLater(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "api"}
	store := &deletionTestStore{
		repository: repository,
		inventory: []registrydomain.ManifestInventory{{
			RepositoryID: repository.ID, Digest: "sha256:missing", State: constants.InventoryStateMissing,
		}},
	}
	distribution := &missingProbeDistribution{}
	service := NewInventoryService(store)
	service.SetDistribution(distribution)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	for range 2 {
		if _, err := service.Reconcile(context.Background(), projectID, "payments", "api"); err != nil {
			t.Fatal(err)
		}
	}
	if distribution.resolveCalls != 1 {
		t.Fatalf("missing manifest resolve calls = %d, want 1 within backoff", distribution.resolveCalls)
	}

	now = now.Add(missingManifestProbeInterval)
	distribution.available = true
	if _, err := service.Reconcile(context.Background(), projectID, "payments", "api"); err != nil {
		t.Fatal(err)
	}
	if distribution.resolveCalls != 2 || distribution.fetchCalls != 1 {
		t.Fatalf("eligible missing manifest was not reprobed and fetched: resolve=%d fetch=%d", distribution.resolveCalls, distribution.fetchCalls)
	}
}

func TestListPageSearchesInventoryForTheResolvedRepository(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "api"}
	store := &deletionTestStore{
		repository: repository,
		inventory:  []registrydomain.ManifestInventory{{RepositoryID: repository.ID, Digest: "sha256:aaaa"}},
	}
	service := NewInventoryService(store)

	page, err := service.ListPage(context.Background(), projectID, "api", "aaaa", foundation.PageRequest{Limit: 10})
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Digest != "sha256:aaaa" {
		t.Fatalf("page = %#v", page)
	}
}

func TestListPagePropagatesUnknownRepositoryError(t *testing.T) {
	store := &deletionTestStore{repository: &registrydomain.Repository{ID: foundation.NewID(), ProjectID: foundation.NewID(), Name: "other"}}
	service := NewInventoryService(store)

	if _, err := service.ListPage(context.Background(), foundation.NewID(), "missing", "", foundation.PageRequest{Limit: 10}); err == nil {
		t.Fatal("expected an error for an unknown repository")
	}
}

func (s *deletionTestStore) FindManifestInventory(_ context.Context, repositoryID foundation.ID, digest string) (*registrydomain.ManifestInventory, error) {
	for i := range s.inventory {
		if s.inventory[i].RepositoryID == repositoryID && s.inventory[i].Digest == digest {
			return &s.inventory[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *deletionTestStore) ListManifestReferrersPage(_ context.Context, repositoryID foundation.ID, subjectDigest string, _ foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	result := foundation.PageResult[registrydomain.ManifestInventory]{Items: []registrydomain.ManifestInventory{}}
	for _, item := range s.inventory {
		if item.RepositoryID == repositoryID && item.SubjectDigest == subjectDigest {
			result.Items = append(result.Items, item)
		}
	}
	return result, nil
}

func TestFindAndListReferrersResolveTheProjectRepository(t *testing.T) {
	projectID := foundation.NewID()
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "api"}
	store := &deletionTestStore{
		repository: repository,
		inventory: []registrydomain.ManifestInventory{
			{RepositoryID: repository.ID, Digest: "sha256:subject"},
			{RepositoryID: repository.ID, Digest: "sha256:sbom", SubjectDigest: "sha256:subject"},
		},
	}
	service := NewInventoryService(store)
	ctx := context.Background()

	found, err := service.Find(ctx, projectID, "api", "sha256:subject")
	if err != nil || found.Digest != "sha256:subject" {
		t.Fatalf("find = %#v, err = %v", found, err)
	}
	referrers, err := service.ListReferrersPage(ctx, projectID, "api", "sha256:subject", foundation.PageRequest{Limit: 10})
	if err != nil || len(referrers.Items) != 1 || referrers.Items[0].Digest != "sha256:sbom" {
		t.Fatalf("referrers = %#v, err = %v", referrers, err)
	}
	if _, err := service.Find(ctx, foundation.NewID(), "api", "sha256:subject"); err == nil {
		t.Fatal("expected a repository from another project to be rejected")
	}
	if _, err := service.ListReferrersPage(ctx, projectID, "missing", "sha256:subject", foundation.PageRequest{Limit: 10}); err == nil {
		t.Fatal("expected an unknown repository to be rejected")
	}
}
