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
	// pendingObservations maps repository@reference to an imported digest
	// whose inventory observation has not been recorded yet.
	pendingObservations sync.Map
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
		upstream: upstream, observe: observe, logger: logger, timeout: 15 * time.Minute,
	}
}

func (s *PullThroughService) Ensure(ctx context.Context, fullRepository, reference string) (bool, error) {
	projectSlug, repository, ok := strings.Cut(fullRepository, "/")
	if !ok || projectSlug == "" || repository == "" {
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
	existing, err := s.repositories.FindByName(ctx, project.ID, repository)
	if err != nil {
		return true, err
	}
	if existing != nil && existing.Status == constants.RepositoryStatusArchived {
		return true, s.serveArchived(ctx, fullRepository, reference)
	}
	return true, s.ensure(ctx, pullThroughTarget{
		projectID: project.ID, upstream: project.Upstream, repository: repository,
		fullRepository: fullRepository, reference: reference, registered: existing != nil,
	})
}

type pullThroughTarget struct {
	projectID      foundation.ID
	upstream       string
	repository     string
	fullRepository string
	reference      string
	registered     bool
}

func (t pullThroughTarget) key() string {
	return t.fullRepository + "@" + t.reference
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
		return s.settle(ctx, target)
	}
	sourceRepository := upstreamRepository(target.upstream, target.repository)
	resolveCtx, cancelResolve := context.WithTimeout(ctx, s.timeout)
	remoteDigest, err := s.upstream.Resolve(resolveCtx, target.upstream, sourceRepository, target.reference)
	cancelResolve()
	if err != nil {
		s.logger.ErrorContext(ctx, "resolve pull-through upstream manifest", "repository", target.fullRepository, "reference", target.reference, "error", err)
		if localExists {
			s.logger.WarnContext(ctx, "serve local manifest while upstream is unavailable", "repository", target.fullRepository, "reference", target.reference)
			return s.settle(ctx, target)
		}
		return err
	}
	if localExists && localDigest == remoteDigest {
		return s.settle(ctx, target)
	}
	_, err, _ = s.group.Do(target.key(), func() (any, error) {
		// The import is shared by every waiting pull, so it must outlive the
		// leader's request while staying bounded by the pull-through timeout.
		copyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()
		localDigest, localExists, err := s.local.ResolveManifest(copyCtx, target.fullRepository, target.reference)
		if err != nil {
			return nil, err
		}
		if localExists && (localDigest == remoteDigest || strings.Contains(target.reference, ":")) {
			return nil, s.settle(copyCtx, target)
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
				return nil, s.settle(copyCtx, target)
			}
			return nil, err
		}
		if s.observe != nil {
			s.pendingObservations.Store(target.key(), digest)
		}
		return nil, s.settle(copyCtx, target)
	})
	return err
}

// settle runs once local content is confirmed. It creates the logical
// repository only after content exists, so a missing upstream image never
// leaves an empty repository behind, and it retries an inventory observation
// whose earlier write failed. Pending observations live in memory: a restart
// drops them, and a manual inventory reconciliation recovers the gap.
func (s *PullThroughService) settle(ctx context.Context, target pullThroughTarget) error {
	if !target.registered && s.repositories != nil {
		if _, _, err := s.repositories.EnsureFromPullThrough(ctx, target.projectID, target.repository); err != nil {
			return err
		}
	}
	pending, ok := s.pendingObservations.Load(target.key())
	if !ok {
		return nil
	}
	digest, _ := pending.(string)
	if err := s.observe(ctx, target.fullRepository, target.reference, digest, "grom-pull-through"); err != nil {
		s.logger.ErrorContext(ctx, "observe pull-through manifest", "repository", target.fullRepository, "reference", target.reference, "error", err)
		return nil
	}
	s.pendingObservations.CompareAndDelete(target.key(), pending)
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
