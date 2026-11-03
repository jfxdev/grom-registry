package application

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
)

type pullThroughLocal struct {
	mu       sync.Mutex
	digests  map[string]string
	resolves int
}

func (l *pullThroughLocal) ResolveManifest(_ context.Context, repository, reference string) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resolves++
	digest, ok := l.digests[repository+"@"+reference]
	return digest, ok, nil
}

type pullThroughUpstream struct {
	mu               sync.Mutex
	local            *pullThroughLocal
	digest           string
	resolveErr       error
	copyErr          error
	resolves         int
	copies           int
	sourceRepository string
	localRepository  string
	reference        string
	token            string
	copyStarted      chan struct{}
	copyRelease      chan struct{}
}

func (u *pullThroughUpstream) Resolve(_ context.Context, _, repository, _ string) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.resolves++
	return u.digest, u.resolveErr
}

func (u *pullThroughUpstream) Copy(_ context.Context, _, sourceRepository, localRepository, reference, token string) (string, error) {
	u.mu.Lock()
	u.copies++
	u.sourceRepository = sourceRepository
	u.localRepository = localRepository
	u.reference = reference
	u.token = token
	started := u.copyStarted
	release := u.copyRelease
	u.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		<-release
	}
	if u.copyErr != nil {
		return "", u.copyErr
	}
	u.local.mu.Lock()
	u.local.digests[localRepository+"@"+reference] = u.digest
	u.local.digests[localRepository+"@"+u.digest] = u.digest
	u.local.mu.Unlock()
	return u.digest, nil
}

func newPullThroughTestService(t *testing.T, local *pullThroughLocal, upstream *pullThroughUpstream) (*PullThroughService, *TokenService) {
	t.Helper()
	signer, err := signing.LoadOrCreate(filepath.Join(t.TempDir(), "key.pem"), filepath.Join(t.TempDir(), "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	tokens := NewTokenService(nil, nil, signer, time.Minute)
	repositories := NewRepositoryService(&pullThroughRepositoryStore{})
	return NewPullThroughService(nil, repositories, tokens, local, upstream, nil, slog.Default()), tokens
}

// observeWith installs an observer that records successful observations in the
// fake inventory, the way InventoryService.ObservePush does.
func observeWith(service *PullThroughService, observe PullThroughObserver) {
	store := service.repositories.store.(*pullThroughRepositoryStore)
	service.observe = func(ctx context.Context, repository, reference, digest, actor string) error {
		if err := observe(ctx, repository, reference, digest, actor); err != nil {
			return err
		}
		store.markObserved(reference, digest)
		return nil
	}
}

func TestPullThroughTagLifecycleAndDigestImmutability(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1"}
	service, tokens := newPullThroughTestService(t, local, upstream)
	var observations []string
	observeWith(service, func(_ context.Context, repository, reference, digest, actor string) error {
		observations = append(observations, repository+"@"+reference+"="+digest+" by "+actor)
		return nil
	})

	if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamDockerHub, name: "alpine", fullRepository: "dockerhub/alpine", reference: "latest"}); err != nil {
		t.Fatal(err)
	}
	if upstream.sourceRepository != "library/alpine" || upstream.localRepository != "dockerhub/alpine" || upstream.reference != "latest" {
		t.Fatalf("unexpected copy coordinates: %#v", upstream)
	}
	if !tokens.HasAccess(upstream.token, "dockerhub/alpine", constants.RegistryActionPush) {
		t.Fatal("internal copy token cannot write the destination repository")
	}
	if upstream.copies != 1 || len(observations) != 1 {
		t.Fatalf("first pull copies=%d observations=%d", upstream.copies, len(observations))
	}

	if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamDockerHub, name: "alpine", fullRepository: "dockerhub/alpine", reference: "latest"}); err != nil {
		t.Fatal(err)
	}
	if upstream.copies != 1 || upstream.resolves != 2 {
		t.Fatalf("unchanged tag must resolve without copying: copies=%d resolves=%d", upstream.copies, upstream.resolves)
	}

	upstream.digest = "sha256:v2"
	if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamDockerHub, name: "alpine", fullRepository: "dockerhub/alpine", reference: "latest"}); err != nil {
		t.Fatal(err)
	}
	if upstream.copies != 2 || local.digests["dockerhub/alpine@latest"] != "sha256:v2" {
		t.Fatalf("changed tag was not updated: copies=%d local=%q", upstream.copies, local.digests["dockerhub/alpine@latest"])
	}

	upstream.digest = "sha256:upstream-must-not-be-read"
	resolves := upstream.resolves
	if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamDockerHub, name: "alpine", fullRepository: "dockerhub/alpine", reference: "sha256:v2"}); err != nil {
		t.Fatal(err)
	}
	if upstream.resolves != resolves || upstream.copies != 2 {
		t.Fatal("an existing digest reference must remain local and immutable")
	}

	if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamDockerHub, name: "alpine", fullRepository: "dockerhub/alpine", reference: "sha256:missing"}); err != nil {
		t.Fatal(err)
	}
	if upstream.reference != "sha256:missing" || upstream.copies != 3 {
		t.Fatalf("missing digest was not imported exactly: reference=%q copies=%d", upstream.reference, upstream.copies)
	}
}

func TestPullThroughServesLocalTagWhenUpstreamFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		digest  string
		copyErr error
	}{
		{name: "resolve", digest: "sha256:v1", copyErr: nil},
		{name: "copy", digest: "sha256:v2", copyErr: errors.New("upstream write failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			local := &pullThroughLocal{digests: map[string]string{"dockerhub/alpine@latest": "sha256:v1"}}
			upstream := &pullThroughUpstream{local: local, digest: test.digest, resolveErr: errors.New("upstream unavailable"), copyErr: test.copyErr}
			if test.copyErr != nil {
				upstream.resolveErr = nil
			}
			service, _ := newPullThroughTestService(t, local, upstream)
			if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamDockerHub, name: "alpine", fullRepository: "dockerhub/alpine", reference: "latest"}); err != nil {
				t.Fatalf("local fallback failed: %v", err)
			}
			if local.digests["dockerhub/alpine@latest"] != "sha256:v1" {
				t.Fatal("fallback changed the local digest")
			}
		})
	}
}

func TestPullThroughWithoutLocalCopyReturnsUpstreamFailure(t *testing.T) {
	want := errors.New("upstream unavailable")
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, resolveErr: want}
	service, _ := newPullThroughTestService(t, local, upstream)

	if err := service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamGHCR, name: "owner/image", fullRepository: "ghcr/owner/image", reference: "latest"}); !errors.Is(err, want) {
		t.Fatalf("expected upstream error, got %v", err)
	}
	if upstream.copies != 0 {
		t.Fatal("failed resolution must not start a copy")
	}
}

func TestPullThroughConcurrentFirstPullCopiesOnce(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{
		local: local, digest: "sha256:v1", copyStarted: make(chan struct{}), copyRelease: make(chan struct{}),
	}
	service, _ := newPullThroughTestService(t, local, upstream)
	observations := 0
	observeWith(service, func(context.Context, string, string, string, string) error {
		observations++
		return nil
	})

	const callers = 8
	errs := make(chan error, callers)
	for range callers {
		go func() {
			errs <- service.ensure(context.Background(), pullThroughTarget{upstream: constants.ProjectUpstreamGHCR, name: "owner/image", fullRepository: "ghcr/owner/image", reference: "latest"})
		}()
	}
	select {
	case <-upstream.copyStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first copy did not start")
	}
	close(upstream.copyRelease)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent pull: %v", err)
		}
	}
	if upstream.copies != 1 || observations != 1 {
		t.Fatalf("concurrent imports duplicated work: copies=%d observations=%d", upstream.copies, observations)
	}
}

type pullThroughProjectStore struct {
	projectdomain.Repository
	project *projectdomain.Project
}

func (s *pullThroughProjectStore) FindProjectBySlug(context.Context, string) (*projectdomain.Project, error) {
	if s.project == nil {
		return nil, sql.ErrNoRows
	}
	return s.project, nil
}

type pullThroughRepositoryStore struct {
	registrydomain.Store
	mu         sync.Mutex
	repository *registrydomain.Repository
	observed   map[string]bool
	hasRows    bool
}

func (s *pullThroughRepositoryStore) markObserved(reference, digest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observed == nil {
		s.observed = map[string]bool{}
	}
	s.observed[reference+"="+digest] = true
}

func (s *pullThroughRepositoryStore) ManifestObserved(_ context.Context, _ foundation.ID, tag, digest string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reference := tag
	if reference == "" {
		reference = digest
	}
	return s.observed[reference+"="+digest], nil
}

func (s *pullThroughRepositoryStore) ProjectHasRepositories(context.Context, foundation.ID) (bool, error) {
	return s.hasRows, nil
}

func (s *pullThroughRepositoryStore) FindRepository(context.Context, foundation.ID, string) (*registrydomain.Repository, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repository == nil {
		return nil, sql.ErrNoRows
	}
	return s.repository, nil
}

func (s *pullThroughRepositoryStore) EnsureRepository(_ context.Context, repository *registrydomain.Repository) (*registrydomain.Repository, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repository != nil {
		return s.repository, false, nil
	}
	s.repository = repository
	return repository, true, nil
}

type pullThroughAudit struct {
	err     error
	actions []string
}

func (a *pullThroughAudit) Record(_ context.Context, _ foundation.PrincipalRef, action, _ string, _ foundation.ID, _ map[string]any) error {
	if a.err != nil {
		return a.err
	}
	a.actions = append(a.actions, action)
	return nil
}

func newPullThroughProjectService(t *testing.T, repositoryStore *pullThroughRepositoryStore) (*PullThroughService, *pullThroughUpstream, *pullThroughLocal, *pullThroughProjectStore) {
	t.Helper()
	projectStore := &pullThroughProjectStore{project: &projectdomain.Project{
		ID: foundation.ID("project"), Slug: "dockerhub", Mode: constants.ProjectModePullThrough, Upstream: constants.ProjectUpstreamDockerHub,
	}}
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1"}
	service, _ := newPullThroughTestService(t, local, upstream)
	service.projects = projectapp.New(projectStore)
	service.repositories = NewRepositoryService(repositoryStore)
	return service, upstream, local, projectStore
}

func TestPullThroughEnsureScopesProvisioningToDedicatedProjects(t *testing.T) {
	projectStore := &pullThroughProjectStore{project: &projectdomain.Project{
		ID: foundation.ID("project"), Slug: "dockerhub", Mode: constants.ProjectModePullThrough, Upstream: constants.ProjectUpstreamDockerHub,
	}}
	repositoryStore := &pullThroughRepositoryStore{}
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1"}
	service, _ := newPullThroughTestService(t, local, upstream)
	service.projects = projectapp.New(projectStore)
	service.repositories = NewRepositoryService(repositoryStore)

	handled, err := service.Ensure(context.Background(), "dockerhub/alpine", "latest")
	if err != nil || !handled {
		t.Fatalf("pull-through ensure: handled=%t err=%v", handled, err)
	}
	if repositoryStore.repository == nil || repositoryStore.repository.CreationSource != constants.RepositoryCreationPullThrough ||
		repositoryStore.repository.Name != "alpine" || repositoryStore.repository.ProjectID != projectStore.project.ID {
		t.Fatalf("unexpected logical repository: %#v", repositoryStore.repository)
	}

	projectStore.project.Mode = constants.ProjectModeStandard
	projectStore.project.Upstream = ""
	resolves := upstream.resolves
	handled, err = service.Ensure(context.Background(), "dockerhub/alpine", "latest")
	if err != nil || handled || upstream.resolves != resolves {
		t.Fatalf("standard project was handled as pull-through: handled=%t err=%v", handled, err)
	}

	projectStore.project = nil
	handled, err = service.Ensure(context.Background(), "missing/alpine", "latest")
	if err != nil || handled {
		t.Fatalf("missing project must use the normal registry flow: handled=%t err=%v", handled, err)
	}
}

func TestPullThroughDoesNotCreateRepositoryWhenUpstreamIsMissing(t *testing.T) {
	repositoryStore := &pullThroughRepositoryStore{}
	service, upstream, _, _ := newPullThroughProjectService(t, repositoryStore)
	upstream.resolveErr = ErrUpstreamManifestNotFound

	handled, err := service.Ensure(context.Background(), "dockerhub/missing", "latest")
	if !handled || !errors.Is(err, ErrUpstreamManifestNotFound) {
		t.Fatalf("missing upstream: handled=%t err=%v", handled, err)
	}
	if repositoryStore.repository != nil || upstream.copies != 0 {
		t.Fatalf("missing upstream left a repository or copied: repository=%#v copies=%d", repositoryStore.repository, upstream.copies)
	}
}

func TestPullThroughArchivedRepositoryServesOnlyCachedContent(t *testing.T) {
	repositoryStore := &pullThroughRepositoryStore{repository: &registrydomain.Repository{
		ID: foundation.ID("repository"), Name: "alpine", Status: constants.RepositoryStatusArchived,
	}}
	service, upstream, local, _ := newPullThroughProjectService(t, repositoryStore)
	local.digests["dockerhub/alpine@latest"] = "sha256:cached"
	upstream.digest = "sha256:newer"

	handled, err := service.Ensure(context.Background(), "dockerhub/alpine", "latest")
	if !handled || err != nil {
		t.Fatalf("archived cached pull: handled=%t err=%v", handled, err)
	}
	handled, err = service.Ensure(context.Background(), "dockerhub/alpine", "v2")
	if !handled || !errors.Is(err, ErrRepositoryArchived) {
		t.Fatalf("archived uncached pull: handled=%t err=%v", handled, err)
	}
	if upstream.resolves != 0 || upstream.copies != 0 {
		t.Fatalf("archived repository contacted upstream: resolves=%d copies=%d", upstream.resolves, upstream.copies)
	}
}

func TestPullThroughRejectsInvalidReference(t *testing.T) {
	service, _, _, _ := newPullThroughProjectService(t, &pullThroughRepositoryStore{})
	if handled, err := service.Ensure(context.Background(), "dockerhub/alpine", "a/b"); !handled || err == nil {
		t.Fatalf("invalid reference: handled=%t err=%v", handled, err)
	}
	if handled, err := service.Ensure(context.Background(), "dockerhub", "latest"); handled || err != nil {
		t.Fatalf("repository without project path: handled=%t err=%v", handled, err)
	}
}

func TestPullThroughRegistersRepositoryForExistingLocalContent(t *testing.T) {
	repositoryStore := &pullThroughRepositoryStore{}
	service, upstream, local, _ := newPullThroughProjectService(t, repositoryStore)
	local.digests["dockerhub/alpine@latest"] = "sha256:v1"

	if handled, err := service.Ensure(context.Background(), "dockerhub/alpine", "latest"); !handled || err != nil {
		t.Fatalf("cached pull: handled=%t err=%v", handled, err)
	}
	if repositoryStore.repository == nil || upstream.copies != 0 {
		t.Fatalf("cached content was not registered without copying: repository=%#v copies=%d", repositoryStore.repository, upstream.copies)
	}
}

func TestPullThroughRetriesFailedObservation(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1"}
	service, _ := newPullThroughTestService(t, local, upstream)
	failures := 1
	var observed []string
	observeWith(service, func(_ context.Context, repository, reference, digest, _ string) error {
		if failures > 0 {
			failures--
			return errors.New("inventory unavailable")
		}
		observed = append(observed, repository+"@"+reference+"="+digest)
		return nil
	})
	target := pullThroughTarget{upstream: constants.ProjectUpstreamGHCR, name: "owner/image", fullRepository: "ghcr/owner/image", reference: "latest"}

	if err := service.ensure(context.Background(), target); err != nil {
		t.Fatalf("observation failure must not fail the pull: %v", err)
	}
	if len(observed) != 0 {
		t.Fatal("failed observation was recorded")
	}
	if err := service.ensure(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if upstream.copies != 1 || len(observed) != 1 || observed[0] != "ghcr/owner/image@latest=sha256:v1" {
		t.Fatalf("failed observation was not retried: copies=%d observed=%v", upstream.copies, observed)
	}
	if err := service.ensure(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 {
		t.Fatalf("successful observation was repeated: %v", observed)
	}
}

func TestPullThroughCopySurvivesLeaderCancellation(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1", copyStarted: make(chan struct{}), copyRelease: make(chan struct{})}
	var copyCtx context.Context
	service, tokens := newPullThroughTestService(t, local, upstream)
	service.upstream = &contextCapturingUpstream{pullThroughUpstream: upstream, captured: &copyCtx}
	target := pullThroughTarget{upstream: constants.ProjectUpstreamGHCR, name: "owner/image", fullRepository: "ghcr/owner/image", reference: "latest"}

	started := time.Now()
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leader := make(chan error, 1)
	go func() { leader <- service.ensure(leaderCtx, target) }()
	<-upstream.copyStarted
	cancelLeader()
	close(upstream.copyRelease)
	if err := <-leader; err != nil {
		t.Fatalf("leader cancellation aborted the shared copy: %v", err)
	}
	if deadline, ok := copyCtx.Deadline(); !ok || deadline.Before(started.Add(service.timeout-time.Minute)) {
		t.Fatalf("copy context is not bounded by the pull-through timeout: %v %t", deadline, ok)
	}
	var claims RegistryClaims
	if err := tokens.signer.Verify(upstream.token, &claims, tokens.issuer, tokens.service); err != nil {
		t.Fatal(err)
	}
	if lifetime := claims.ExpiresAt.Sub(claims.IssuedAt.Time); lifetime < service.timeout {
		t.Fatalf("copy token lifetime %s is shorter than the copy timeout %s", lifetime, service.timeout)
	}
}

type contextCapturingUpstream struct {
	*pullThroughUpstream
	captured *context.Context
}

func (u *contextCapturingUpstream) Copy(ctx context.Context, upstream, sourceRepository, localRepository, reference, token string) (string, error) {
	*u.captured = ctx
	return u.pullThroughUpstream.Copy(ctx, upstream, sourceRepository, localRepository, reference, token)
}

func TestEnsureFromPullThroughKeepsRepositoryAndRetriesFailedAudit(t *testing.T) {
	store := &pullThroughRepositoryStore{}
	service := NewRepositoryService(store)
	audit := &pullThroughAudit{err: errors.New("audit unavailable")}
	service.SetAuditRecorder(audit)

	first, created, err := service.EnsureFromPullThrough(context.Background(), foundation.ID("project"), " Alpine ")
	if err == nil || !created || first == nil {
		t.Fatalf("audit failure was not reported for the created repository: created=%t err=%v", created, err)
	}
	if store.repository == nil || store.repository.Name != "alpine" || store.repository.CreationSource != constants.RepositoryCreationPullThrough {
		t.Fatalf("audit failure removed or changed the repository: %#v", store.repository)
	}
	if err := service.RecordPendingCreationAudit(context.Background(), first.ID); err == nil {
		t.Fatal("audit retry succeeded while the recorder still fails")
	}

	audit.err = nil
	if err := service.RecordPendingCreationAudit(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordPendingCreationAudit(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if len(audit.actions) != 1 || audit.actions[0] != constants.AuditRepositoryCreatedFromPullThrough {
		t.Fatalf("creation audit was not recorded exactly once: %v", audit.actions)
	}
	if _, created, err := service.EnsureFromPullThrough(context.Background(), foundation.ID("project"), "alpine"); err != nil || created {
		t.Fatalf("existing repository was recreated: created=%t err=%v", created, err)
	}
	if len(audit.actions) != 1 {
		t.Fatalf("existing repository was audited again: %v", audit.actions)
	}
	if _, _, err := service.EnsureFromPullThrough(context.Background(), foundation.ID("project"), "Bad Name!"); err == nil {
		t.Fatal("invalid repository name was accepted")
	}
}

func TestPullThroughRetriesCreationAuditOnLaterPull(t *testing.T) {
	repositoryStore := &pullThroughRepositoryStore{}
	service, _, _, _ := newPullThroughProjectService(t, repositoryStore)
	audit := &pullThroughAudit{err: errors.New("audit unavailable")}
	service.repositories.SetAuditRecorder(audit)

	if _, err := service.Ensure(context.Background(), "dockerhub/alpine", "latest"); err == nil {
		t.Fatal("creation audit failure was not reported")
	}
	if repositoryStore.repository == nil {
		t.Fatal("audit failure removed the imported repository")
	}
	audit.err = nil
	if _, err := service.Ensure(context.Background(), "dockerhub/alpine", "latest"); err != nil {
		t.Fatal(err)
	}
	if len(audit.actions) != 1 {
		t.Fatalf("later pull did not record the pending creation audit: %v", audit.actions)
	}
}

func TestPullThroughLimitsConcurrentImportsAndGuardsProjectDeletion(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1", copyStarted: make(chan struct{}), copyRelease: make(chan struct{})}
	service, _ := newPullThroughTestService(t, local, upstream)
	service.SetMaxConcurrentImports(1)
	project := foundation.ID("project")

	if busy, err := service.ProjectHasRepositories(context.Background(), project); err != nil || busy {
		t.Fatalf("idle project reported repositories: busy=%t err=%v", busy, err)
	}
	first := make(chan error, 1)
	go func() {
		first <- service.ensure(context.Background(), pullThroughTarget{projectID: project, upstream: constants.ProjectUpstreamGHCR, name: "owner/one", fullRepository: "ghcr/owner/one", reference: "latest"})
	}()
	<-upstream.copyStarted
	if busy, err := service.ProjectHasRepositories(context.Background(), project); err != nil || !busy {
		t.Fatalf("project deletion was not blocked during an import: busy=%t err=%v", busy, err)
	}

	waiting, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	release, err := service.beginImport(waiting, foundation.ID("other"))
	if !errors.Is(err, context.DeadlineExceeded) || release != nil {
		t.Fatalf("import beyond the limit did not wait for a slot: err=%v", err)
	}
	if busy, _ := service.ProjectHasRepositories(context.Background(), foundation.ID("other")); busy {
		t.Fatal("abandoned import kept blocking project deletion")
	}

	close(upstream.copyRelease)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if busy, err := service.ProjectHasRepositories(context.Background(), project); err != nil || busy {
		t.Fatalf("finished import kept blocking project deletion: busy=%t err=%v", busy, err)
	}
	service.repositories.store.(*pullThroughRepositoryStore).hasRows = true
	if busy, err := service.ProjectHasRepositories(context.Background(), project); err != nil || !busy {
		t.Fatalf("repository rows did not block project deletion: busy=%t err=%v", busy, err)
	}
	service.SetMaxConcurrentImports(0)
	if cap(service.imports) != 1 {
		t.Fatal("a non-positive limit replaced the import limit")
	}
}

func TestPullThroughCallerWithNewerDigestDoesNotJoinOlderImport(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1", copyStarted: make(chan struct{}), copyRelease: make(chan struct{})}
	service, _ := newPullThroughTestService(t, local, upstream)
	target := pullThroughTarget{upstream: constants.ProjectUpstreamGHCR, name: "owner/image", fullRepository: "ghcr/owner/image", reference: "latest"}

	first := make(chan error, 1)
	go func() { first <- service.ensure(context.Background(), target) }()
	<-upstream.copyStarted
	upstream.mu.Lock()
	upstream.digest = "sha256:v2"
	secondStarted := make(chan struct{})
	upstream.copyStarted = secondStarted
	upstream.mu.Unlock()

	second := make(chan error, 1)
	go func() { second <- service.ensure(context.Background(), target) }()
	select {
	case <-secondStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("caller with a newer digest did not start its own import")
	}
	close(upstream.copyRelease)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	if upstream.copies != 2 {
		t.Fatalf("caller with a newer digest joined the older import: copies=%d", upstream.copies)
	}
}

func TestFindByNameReturnsNilForMissingRepository(t *testing.T) {
	store := &pullThroughRepositoryStore{}
	service := NewRepositoryService(store)
	repository, err := service.FindByName(context.Background(), foundation.ID("project"), "alpine")
	if err != nil || repository != nil {
		t.Fatalf("missing repository: repository=%#v err=%v", repository, err)
	}
	store.repository = &registrydomain.Repository{Name: "alpine"}
	repository, err = service.FindByName(context.Background(), foundation.ID("project"), "alpine")
	if err != nil || repository == nil {
		t.Fatalf("existing repository: repository=%#v err=%v", repository, err)
	}
}
