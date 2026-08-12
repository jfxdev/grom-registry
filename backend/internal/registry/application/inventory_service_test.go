package application

import (
	"context"
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
