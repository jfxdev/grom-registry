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

type lifecycleExecutionStore struct {
	registrydomain.Store
	repository       *registrydomain.Repository
	preview          *registrydomain.LifecyclePreview
	inventory        []registrydomain.ManifestInventory
	marked           []string
	completeRunErr   error
	previewStatusErr error
}

func (s *lifecycleExecutionStore) ListLifecycleRunsPage(_ context.Context, _ foundation.ID, _ foundation.PageRequest) (foundation.PageResult[registrydomain.LifecycleRun], error) {
	return foundation.PageResult[registrydomain.LifecycleRun]{Items: []registrydomain.LifecycleRun{{ID: foundation.NewID(), RepositoryID: s.repository.ID}}}, nil
}

func (s *lifecycleExecutionStore) FindRepository(
	context.Context,
	foundation.ID,
	string,
) (*registrydomain.Repository, error) {
	return s.repository, nil
}

func (s *lifecycleExecutionStore) FindRepositoryByID(
	context.Context,
	foundation.ID,
) (*registrydomain.Repository, error) {
	return s.repository, nil
}

func (s *lifecycleExecutionStore) FindLifecyclePreview(
	context.Context,
	foundation.ID,
) (*registrydomain.LifecyclePreview, error) {
	return s.preview, nil
}

func (s *lifecycleExecutionStore) UpsertManifestObservation(
	context.Context,
	foundation.ID,
	registrydomain.ManifestObservation,
	time.Time,
) error {
	return nil
}

func (s *lifecycleExecutionStore) CompleteInventoryReconciliation(
	context.Context,
	foundation.ID,
	[]string,
	[]string,
	time.Time,
) error {
	return nil
}

func (s *lifecycleExecutionStore) ListManifestInventory(
	context.Context,
	foundation.ID,
) ([]registrydomain.ManifestInventory, error) {
	return s.inventory, nil
}

func (s *lifecycleExecutionStore) SaveRepositoryProfile(
	context.Context,
	*registrydomain.Repository,
) error {
	return nil
}

func (s *lifecycleExecutionStore) AcquireLifecycleLock(
	context.Context,
	foundation.ID,
	foundation.ID,
	time.Time,
) error {
	return nil
}

func (s *lifecycleExecutionStore) ReleaseLifecycleLock(
	context.Context,
	foundation.ID,
	foundation.ID,
) error {
	return nil
}

func (s *lifecycleExecutionStore) CreateLifecycleRunFromPreview(
	context.Context,
	*registrydomain.LifecycleRun,
) error {
	return nil
}

func (s *lifecycleExecutionStore) UpdateLifecycleRunItem(
	context.Context,
	*registrydomain.LifecycleRunItem,
) error {
	return nil
}

func (s *lifecycleExecutionStore) CompleteLifecycleRun(
	context.Context,
	foundation.ID,
	string,
	time.Time,
) error {
	return s.completeRunErr
}

func (s *lifecycleExecutionStore) SetLifecyclePreviewStatus(
	context.Context,
	foundation.ID,
	string,
) error {
	return s.previewStatusErr
}

func (s *lifecycleExecutionStore) MarkManifestDeleted(
	_ context.Context,
	_ foundation.ID,
	digest string,
	_ time.Time,
) error {
	s.marked = append(s.marked, digest)
	return nil
}

type lifecycleExecutionDistribution struct {
	manifests     map[string]*ManifestMetadata
	listTagsCalls int
	resolveCalls  int
	referrerCalls int
	deletionCalls int
}

type rejectingLifecycleAudit struct{}

func (rejectingLifecycleAudit) Record(
	context.Context,
	foundation.PrincipalRef,
	string,
	string,
	foundation.ID,
	map[string]any,
) error {
	return errors.New("audit unavailable")
}

func (d *lifecycleExecutionDistribution) ListRepositoryTags(
	context.Context,
	string,
) ([]string, error) {
	d.listTagsCalls++
	return []string{"release-one", "release-two"}, nil
}

func (d *lifecycleExecutionDistribution) ResolveManifest(
	_ context.Context,
	_, reference string,
) (string, bool, error) {
	d.resolveCalls++
	manifest, exists := d.manifests[reference]
	if !exists {
		return "", false, nil
	}
	return manifest.Digest, true, nil
}

func (d *lifecycleExecutionDistribution) FetchManifest(
	_ context.Context,
	_, reference string,
) (*ManifestMetadata, error) {
	return d.manifests[reference], nil
}

func (d *lifecycleExecutionDistribution) ListReferrers(
	context.Context,
	string,
	string,
) ([]ManifestDescriptor, error) {
	d.referrerCalls++
	return nil, nil
}

func (d *lifecycleExecutionDistribution) DeleteManifest(
	context.Context,
	string,
	string,
) error {
	d.deletionCalls++
	return nil
}

func TestEvaluateLifecycleProtectsAliasesAndReferrers(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	fourteen, one, two := 14, 1, 2
	policies := []registrydomain.Policy{{
		Type: constants.RepositoryPolicyRetention, Enabled: true,
		TagPatterns: []string{"pr-*"}, ExpireAfterDays: &fourteen,
		KeepLast: &one, UntaggedGraceDays: &two,
	}, {
		Type: constants.RepositoryPolicyTagProtection, Enabled: true,
		TagPatterns: []string{"prod"}, PreventDeletion: true, ExcludeFromLifecycle: true,
	}}
	at := func(days int) *time.Time {
		value := now.Add(-time.Duration(days) * 24 * time.Hour)
		return &value
	}
	untagged := now.Add(-3 * 24 * time.Hour)
	inventory := []registrydomain.ManifestInventory{
		{ID: foundation.NewID(), Digest: "sha256:new", Tags: []string{"pr-new"}, State: constants.InventoryStateActive, FirstSeenAt: *at(1), LastPushedAt: at(1)},
		{ID: foundation.NewID(), Digest: "sha256:old", Tags: []string{"pr-old"}, State: constants.InventoryStateActive, FirstSeenAt: *at(30), LastPushedAt: at(30)},
		{ID: foundation.NewID(), Digest: "sha256:protected", Tags: []string{"pr-prod", "prod"}, State: constants.InventoryStateActive, FirstSeenAt: *at(40), LastPushedAt: at(40)},
		{ID: foundation.NewID(), Digest: "sha256:untagged", Tags: []string{}, State: constants.InventoryStateUntagged, FirstSeenAt: *at(10), UntaggedAt: &untagged},
		{ID: foundation.NewID(), Digest: "sha256:subject", Tags: []string{"pr-subject"}, State: constants.InventoryStateActive, FirstSeenAt: *at(50), LastPushedAt: at(50)},
		{ID: foundation.NewID(), Digest: "sha256:sbom", SubjectDigest: "sha256:subject", Tags: []string{}, State: constants.InventoryStateUntagged, FirstSeenAt: *at(10), UntaggedAt: &untagged},
	}

	items, err := evaluateLifecycle(policies, inventory, now)
	if err != nil {
		t.Fatal(err)
	}
	decisions := map[string]string{}
	for _, item := range items {
		decisions[item.Digest] = item.Decision
	}
	expected := map[string]string{
		"sha256:new":       constants.LifecycleDecisionRetained,
		"sha256:old":       constants.LifecycleDecisionEligible,
		"sha256:protected": constants.LifecycleDecisionBlocked,
		"sha256:untagged":  constants.LifecycleDecisionEligible,
		"sha256:subject":   constants.LifecycleDecisionBlocked,
		"sha256:sbom":      constants.LifecycleDecisionBlocked,
	}
	for digest, decision := range expected {
		if decisions[digest] != decision {
			t.Fatalf("%s: expected %s, got %s", digest, decision, decisions[digest])
		}
	}
}

func TestEvaluateLifecycleRequiresRetentionPolicy(t *testing.T) {
	if _, err := evaluateLifecycle(nil, nil, time.Now()); err == nil {
		t.Fatal("expected missing retention policy to fail")
	}
}

func TestEvaluateLifecycleIgnoresProtectionWithoutTagPatterns(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	expireAfterDays := 1
	pushedAt := now.Add(-48 * time.Hour)
	policies := []registrydomain.Policy{{
		Type: constants.RepositoryPolicyRetention, Enabled: true,
		TagPatterns: []string{"release-*"}, ExpireAfterDays: &expireAfterDays,
	}, {
		Type: constants.RepositoryPolicyTagProtection, Enabled: true,
		PreventDeletion: true, ExcludeFromLifecycle: true,
	}}
	inventory := []registrydomain.ManifestInventory{{
		ID: foundation.NewID(), Digest: "sha256:release", Tags: []string{"release-old"},
		State: constants.InventoryStateActive, FirstSeenAt: pushedAt, LastPushedAt: &pushedAt,
	}}

	items, err := evaluateLifecycle(policies, inventory, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Decision != constants.LifecycleDecisionEligible {
		t.Fatalf("expected empty protection patterns not to block the manifest: %#v", items)
	}
}

func TestLifecycleExecutionReconcilesOnceAndRevalidatesEachCandidate(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	projectID := foundation.NewID()
	repositoryID := foundation.NewID()
	previewID := foundation.NewID()
	expireAfterDays := 1
	policies := []registrydomain.Policy{{
		Type: constants.RepositoryPolicyRetention, Enabled: true,
		TagPatterns: []string{"release-*"}, ExpireAfterDays: &expireAfterDays,
	}}
	inventory := []registrydomain.ManifestInventory{{
		ID: foundation.NewID(), RepositoryID: repositoryID,
		Digest: "sha256:one", Tags: []string{"release-one"},
		State: constants.InventoryStateActive, FirstSeenAt: now.Add(-48 * time.Hour),
	}, {
		ID: foundation.NewID(), RepositoryID: repositoryID,
		Digest: "sha256:two", Tags: []string{"release-two"},
		State: constants.InventoryStateActive, FirstSeenAt: now.Add(-72 * time.Hour),
	}}
	previewItems, err := evaluateLifecycle(policies, inventory, now)
	if err != nil {
		t.Fatal(err)
	}
	store := &lifecycleExecutionStore{
		repository: &registrydomain.Repository{
			ID: repositoryID, ProjectID: projectID, Name: "api", Policies: policies,
		},
		preview: &registrydomain.LifecyclePreview{
			ID: previewID, RepositoryID: repositoryID,
			Status: constants.LifecyclePreviewReady, ExpiresAt: now.Add(time.Hour),
			Items: previewItems,
		},
		inventory: inventory,
	}
	distribution := &lifecycleExecutionDistribution{manifests: map[string]*ManifestMetadata{
		"release-one": {Digest: "sha256:one"},
		"sha256:one":  {Digest: "sha256:one"},
		"release-two": {Digest: "sha256:two"},
		"sha256:two":  {Digest: "sha256:two"},
	}}
	inventoryService := NewInventoryService(store)
	inventoryService.SetDistribution(distribution)
	service := NewLifecycleService(store, inventoryService, distribution, nil)
	service.now = func() time.Time { return now }

	run, err := service.Execute(
		context.Background(), projectID, "payments", previewID, "cleanup",
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != constants.LifecycleRunCompleted || distribution.deletionCalls != 2 ||
		len(store.marked) != 2 {
		t.Fatalf("unexpected lifecycle execution: %#v", run)
	}
	if distribution.listTagsCalls != 3 {
		t.Fatalf("expected one reconciliation listing and one live-tag listing per deletion, got %d", distribution.listTagsCalls)
	}
	if distribution.resolveCalls != 10 {
		t.Fatalf("expected reconciliation, candidate revalidation, and immediate pre-delete checks, got %d calls", distribution.resolveCalls)
	}
	if distribution.referrerCalls != 4 {
		t.Fatalf("expected reconciliation and immediate referrer checks, got %d calls", distribution.referrerCalls)
	}

	for _, test := range []struct {
		name             string
		completeRunErr   error
		previewStatusErr error
	}{
		{name: "complete run", completeRunErr: errors.New("complete run")},
		{name: "set preview status", previewStatusErr: errors.New("set preview status")},
	} {
		t.Run("returns completed run when "+test.name+" fails", func(t *testing.T) {
			store.completeRunErr = test.completeRunErr
			store.previewStatusErr = test.previewStatusErr
			expectedErr := test.completeRunErr
			if expectedErr == nil {
				expectedErr = test.previewStatusErr
			}

			failedRun, executeErr := service.Execute(
				context.Background(), projectID, "payments", previewID, "cleanup",
				foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
			)

			if !errors.Is(executeErr, expectedErr) {
				t.Fatalf("expected %v, got %v", expectedErr, executeErr)
			}
			if failedRun == nil || failedRun.Status != constants.LifecycleRunCompleted ||
				failedRun.CompletedAt == nil {
				t.Fatalf("expected populated terminal run, got %#v", failedRun)
			}
			store.completeRunErr = nil
			store.previewStatusErr = nil
		})
	}
}

func TestLifecycleExecutionDoesNotReachDistributionWhenStartAuditFails(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	projectID := foundation.NewID()
	repositoryID := foundation.NewID()
	previewID := foundation.NewID()
	expireAfterDays := 1
	policies := []registrydomain.Policy{{
		Type: constants.RepositoryPolicyRetention, Enabled: true,
		TagPatterns: []string{"release-*"}, ExpireAfterDays: &expireAfterDays,
	}}
	inventory := []registrydomain.ManifestInventory{{
		ID: foundation.NewID(), RepositoryID: repositoryID,
		Digest: "sha256:one", Tags: []string{"release-one"},
		State: constants.InventoryStateActive, FirstSeenAt: now.Add(-48 * time.Hour),
	}}
	previewItems, err := evaluateLifecycle(policies, inventory, now)
	if err != nil {
		t.Fatal(err)
	}
	store := &lifecycleExecutionStore{
		repository: &registrydomain.Repository{
			ID: repositoryID, ProjectID: projectID, Name: "api", Policies: policies,
		},
		preview: &registrydomain.LifecyclePreview{
			ID: previewID, RepositoryID: repositoryID,
			Status: constants.LifecyclePreviewReady, ExpiresAt: now.Add(time.Hour),
			Items: previewItems,
		},
		inventory: inventory,
	}
	distribution := &lifecycleExecutionDistribution{manifests: map[string]*ManifestMetadata{
		"release-one": {Digest: "sha256:one"},
		"sha256:one":  {Digest: "sha256:one"},
	}}
	inventoryService := NewInventoryService(store)
	inventoryService.SetDistribution(distribution)
	service := NewLifecycleService(store, inventoryService, distribution, rejectingLifecycleAudit{})
	service.now = func() time.Time { return now }

	run, err := service.Execute(
		context.Background(), projectID, "payments", previewID, "cleanup",
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: foundation.NewID()},
	)

	if err == nil {
		t.Fatal("expected audit failure")
	}
	if run != nil || distribution.deletionCalls != 0 {
		t.Fatalf("lifecycle execution reached Distribution despite audit failure: run=%#v deletions=%d", run, distribution.deletionCalls)
	}
}

func TestLifecycleListRunsPageAddsRepositoryName(t *testing.T) {
	repository := &registrydomain.Repository{ID: foundation.NewID(), ProjectID: foundation.NewID(), Name: "api"}
	service := NewLifecycleService(&lifecycleExecutionStore{repository: repository}, nil, nil, nil)
	page, err := service.ListRunsPage(context.Background(), repository.ProjectID, "api", foundation.PageRequest{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Repository != "api" {
		t.Fatalf("unexpected page: %#v", page)
	}
}
