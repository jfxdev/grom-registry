package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"golang.org/x/sync/singleflight"
)

var ErrUpstreamManifestNotFound = errors.New("upstream manifest not found")

type UpstreamCopier interface {
	Resolve(ctx context.Context, upstream, repository, reference string) (string, error)
	Copy(ctx context.Context, upstream, sourceRepository, localRepository, reference, token string) (string, error)
}

type LocalManifestResolver interface {
	ResolveManifest(ctx context.Context, repository, reference string) (string, bool, error)
}

type PullThroughObserver func(ctx context.Context, repository, reference, digest, actor string) error

type PullThroughService struct {
	projects     *projectapp.Service
	repositories *RepositoryService
	tokens       *TokenService
	local        LocalManifestResolver
	upstream     UpstreamCopier
	observe      PullThroughObserver
	logger       *slog.Logger
	timeout      time.Duration
	group        singleflight.Group
	imports      chan struct{}

	importsMu     sync.Mutex
	activeImports map[foundation.ID]int
}

func NewPullThroughService(
	projects *projectapp.Service,
	repositories *RepositoryService,
	tokens *TokenService,
	local LocalManifestResolver,
	upstream UpstreamCopier,
	observe PullThroughObserver,
	logger *slog.Logger,
) *PullThroughService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PullThroughService{
		projects: projects, repositories: repositories, tokens: tokens, local: local,
		upstream: upstream, observe: observe, logger: logger, timeout: constants.PullThroughImportTimeout,
		imports:       make(chan struct{}, constants.DefaultPullThroughMaxConcurrentImports),
		activeImports: map[foundation.ID]int{},
	}
}

// SetMaxConcurrentImports bounds how many upstream copies run at once. Further
// imports wait for a free slot within the import timeout.
func (s *PullThroughService) SetMaxConcurrentImports(limit int) {
	if limit > 0 {
		s.imports = make(chan struct{}, limit)
	}
}

// ProjectHasRepositories extends the repository deletion guard with in-flight
// imports, whose logical repository is registered only after the copy.
func (s *PullThroughService) ProjectHasRepositories(ctx context.Context, projectID foundation.ID) (bool, error) {
	s.importsMu.Lock()
	active := s.activeImports[projectID] > 0
	s.importsMu.Unlock()
	if active {
		return true, nil
	}
	return s.repositories.ProjectHasRepositories(ctx, projectID)
}

func (s *PullThroughService) Ensure(ctx context.Context, fullRepository, reference string) (bool, error) {
	projectSlug, name, ok := strings.Cut(fullRepository, "/")
	if !ok || projectSlug == "" || name == "" {
		return false, nil
	}
	if err := validatePullThroughReference(reference); err != nil {
		return true, err
	}
	project, err := s.projects.Find(ctx, projectSlug)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if project.Mode != constants.ProjectModePullThrough {
		return false, nil
	}
	repository, err := s.repositories.FindByName(ctx, project.ID, name)
	if err != nil {
		return true, err
	}
	if repository != nil && repository.Status == constants.RepositoryStatusArchived {
		return true, s.serveArchived(ctx, fullRepository, reference)
	}
	return true, s.ensure(ctx, pullThroughTarget{
		projectID: project.ID, upstream: project.Upstream, name: name,
		fullRepository: fullRepository, reference: reference, repository: repository,
	})
}

type pullThroughTarget struct {
	projectID      foundation.ID
	upstream       string
	name           string
	fullRepository string
	reference      string
	// repository is the existing logical repository, or nil before the first import.
	repository *registrydomain.Repository
}

// serveArchived keeps cached content pullable but never imports new content,
// because archival blocks every future write to the repository.
func (s *PullThroughService) serveArchived(ctx context.Context, fullRepository, reference string) error {
	_, exists, err := s.local.ResolveManifest(ctx, fullRepository, reference)
	if err != nil {
		return err
	}
	if !exists {
		return ErrRepositoryArchived
	}
	return nil
}

func (s *PullThroughService) ensure(ctx context.Context, target pullThroughTarget) error {
	localDigest, localExists, err := s.local.ResolveManifest(ctx, target.fullRepository, target.reference)
	if err != nil {
		return err
	}
	if localExists && strings.Contains(target.reference, ":") {
		return s.settle(ctx, target, localDigest)
	}
	sourceRepository := upstreamRepository(target.upstream, target.name)
	resolveCtx, cancelResolve := context.WithTimeout(ctx, s.timeout)
	remoteDigest, err := s.upstream.Resolve(resolveCtx, target.upstream, sourceRepository, target.reference)
	cancelResolve()
	if err != nil {
		s.logger.ErrorContext(ctx, "resolve pull-through upstream manifest", "repository", target.fullRepository, "reference", target.reference, "error", err)
		if localExists {
			s.logger.WarnContext(ctx, "serve local manifest while upstream is unavailable", "repository", target.fullRepository, "reference", target.reference)
			return s.settle(ctx, target, localDigest)
		}
		return err
	}
	if localExists && localDigest == remoteDigest {
		return s.settle(ctx, target, localDigest)
	}
	// The resolved digest is part of the key so a caller that saw a newer
	// digest never joins an import of an older one.
	key := target.fullRepository + "@" + target.reference + "=" + remoteDigest
	_, err, _ = s.group.Do(key, func() (any, error) {
		// The import is shared by every waiting pull, so it must outlive the
		// leader's request while staying bounded by the pull-through timeout.
		copyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()
		release, err := s.beginImport(copyCtx, target.projectID)
		if err != nil {
			return nil, err
		}
		defer release()
		localDigest, localExists, err := s.local.ResolveManifest(copyCtx, target.fullRepository, target.reference)
		if err != nil {
			return nil, err
		}
		if localExists && (localDigest == remoteDigest || strings.Contains(target.reference, ":")) {
			return nil, s.settle(copyCtx, target, localDigest)
		}
		token, err := s.tokens.IssueInternalWithTTL("grom-pull-through", []Access{{
			Type: "repository", Name: target.fullRepository, Actions: []string{constants.RegistryActionPull, constants.RegistryActionPush},
		}}, s.timeout+time.Minute)
		if err != nil {
			return nil, err
		}
		digest, err := s.upstream.Copy(copyCtx, target.upstream, sourceRepository, target.fullRepository, target.reference, token)
		if err != nil {
			if localExists {
				s.logger.WarnContext(copyCtx, "serve local manifest while upstream copy is unavailable", "repository", target.fullRepository, "reference", target.reference)
				return nil, s.settle(copyCtx, target, localDigest)
			}
			return nil, err
		}
		return nil, s.settle(copyCtx, target, digest)
	})
	return err
}

// beginImport marks the project as importing, which blocks its deletion, and
// waits for a free import slot. The returned function releases both.
func (s *PullThroughService) beginImport(ctx context.Context, projectID foundation.ID) (func(), error) {
	s.importsMu.Lock()
	s.activeImports[projectID]++
	s.importsMu.Unlock()
	finish := func() {
		s.importsMu.Lock()
		if s.activeImports[projectID]--; s.activeImports[projectID] <= 0 {
			delete(s.activeImports, projectID)
		}
		s.importsMu.Unlock()
	}
	select {
	case s.imports <- struct{}{}:
		return func() {
			<-s.imports
			finish()
		}, nil
	case <-ctx.Done():
		finish()
		return nil, ctx.Err()
	}
}

// settle runs once local content is confirmed. It creates the logical
// repository only after content exists, so a missing upstream image never
// leaves an empty repository behind, and it observes the manifest whenever the
// inventory does not record it yet. Checking the inventory, rather than
// remembering failures, keeps the retry durable across restarts.
func (s *PullThroughService) settle(ctx context.Context, target pullThroughTarget, digest string) error {
	repository := target.repository
	if repository == nil {
		created, _, err := s.repositories.EnsureFromPullThrough(ctx, target.projectID, target.name)
		if err != nil {
			return err
		}
		repository = created
	} else if err := s.repositories.RecordPendingCreationAudit(ctx, repository.ID); err != nil {
		return err
	}
	if s.observe == nil || digest == "" {
		return nil
	}
	tag := target.reference
	if strings.Contains(tag, ":") {
		tag = ""
	}
	observed, err := s.repositories.ManifestObserved(ctx, repository.ID, tag, digest)
	if err != nil {
		s.logger.ErrorContext(ctx, "inspect pull-through inventory", "repository", target.fullRepository, "reference", target.reference, "error", err)
		return nil
	}
	if observed {
		return nil
	}
	if err := s.observe(ctx, target.fullRepository, target.reference, digest, "grom-pull-through"); err != nil {
		s.logger.ErrorContext(ctx, "observe pull-through manifest", "repository", target.fullRepository, "reference", target.reference, "error", err)
	}
	return nil
}

func upstreamRepository(upstream, repository string) string {
	if upstream == constants.ProjectUpstreamDockerHub && !strings.Contains(repository, "/") {
		return "library/" + repository
	}
	return repository
}

func validatePullThroughReference(reference string) error {
	if reference == "" || strings.Contains(reference, "/") {
		return fmt.Errorf("manifest reference is invalid")
	}
	return nil
}
