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
		started = nil
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
	return NewPullThroughService(nil, nil, tokens, local, upstream, nil, slog.Default()), tokens
}

func TestPullThroughTagLifecycleAndDigestImmutability(t *testing.T) {
	local := &pullThroughLocal{digests: map[string]string{}}
	upstream := &pullThroughUpstream{local: local, digest: "sha256:v1"}
	service, tokens := newPullThroughTestService(t, local, upstream)
	var observations []string
	service.observe = func(_ context.Context, repository, reference, digest, actor string) error {
		observations = append(observations, repository+"@"+reference+"="+digest+" by "+actor)
		return nil
	}

	if err := service.ensure(context.Background(), constants.ProjectUpstreamDockerHub, "alpine", "dockerhub/alpine", "latest"); err != nil {
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

	if err := service.ensure(context.Background(), constants.ProjectUpstreamDockerHub, "alpine", "dockerhub/alpine", "latest"); err != nil {
		t.Fatal(err)
	}
	if upstream.copies != 1 || upstream.resolves != 2 {
		t.Fatalf("unchanged tag must resolve without copying: copies=%d resolves=%d", upstream.copies, upstream.resolves)
	}

	upstream.digest = "sha256:v2"
	if err := service.ensure(context.Background(), constants.ProjectUpstreamDockerHub, "alpine", "dockerhub/alpine", "latest"); err != nil {
		t.Fatal(err)
	}
	if upstream.copies != 2 || local.digests["dockerhub/alpine@latest"] != "sha256:v2" {
		t.Fatalf("changed tag was not updated: copies=%d local=%q", upstream.copies, local.digests["dockerhub/alpine@latest"])
	}

	upstream.digest = "sha256:upstream-must-not-be-read"
	resolves := upstream.resolves
	if err := service.ensure(context.Background(), constants.ProjectUpstreamDockerHub, "alpine", "dockerhub/alpine", "sha256:v2"); err != nil {
		t.Fatal(err)
	}
	if upstream.resolves != resolves || upstream.copies != 2 {
		t.Fatal("an existing digest reference must remain local and immutable")
	}

	if err := service.ensure(context.Background(), constants.ProjectUpstreamDockerHub, "alpine", "dockerhub/alpine", "sha256:missing"); err != nil {
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
			if err := service.ensure(context.Background(), constants.ProjectUpstreamDockerHub, "alpine", "dockerhub/alpine", "latest"); err != nil {
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

	if err := service.ensure(context.Background(), constants.ProjectUpstreamGHCR, "owner/image", "ghcr/owner/image", "latest"); !errors.Is(err, want) {
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
	service.observe = func(context.Context, string, string, string, string) error {
		observations++
		return nil
	}

	const callers = 8
	errs := make(chan error, callers)
	for range callers {
		go func() {
			errs <- service.ensure(context.Background(), constants.ProjectUpstreamGHCR, "owner/image", "ghcr/owner/image", "latest")
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
	repository *registrydomain.Repository
}

func (s *pullThroughRepositoryStore) EnsureRepository(_ context.Context, repository *registrydomain.Repository) (*registrydomain.Repository, bool, error) {
	if s.repository != nil {
		return s.repository, false, nil
	}
	s.repository = repository
	return repository, true, nil
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
