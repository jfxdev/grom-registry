package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
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
	if _, _, err := s.repositories.EnsureFromPullThrough(ctx, project.ID, repository); err != nil {
		return true, err
	}
	return true, s.ensure(ctx, project.Upstream, repository, fullRepository, reference)
}

func (s *PullThroughService) ensure(ctx context.Context, upstream, repository, fullRepository, reference string) error {
	localDigest, localExists, err := s.local.ResolveManifest(ctx, fullRepository, reference)
	if err != nil {
		return err
	}
	if localExists && strings.Contains(reference, ":") {
		return nil
	}
	sourceRepository := upstreamRepository(upstream, repository)
	remoteDigest, err := s.upstream.Resolve(ctx, upstream, sourceRepository, reference)
	if err != nil {
		s.logger.ErrorContext(ctx, "resolve pull-through upstream manifest", "repository", fullRepository, "reference", reference, "error", err)
		if localExists {
			s.logger.WarnContext(ctx, "serve local manifest while upstream is unavailable", "repository", fullRepository, "reference", reference)
			return nil
		}
		return err
	}
	if localExists && localDigest == remoteDigest {
		return nil
	}
	_, err, _ = s.group.Do(fullRepository+"@"+reference, func() (any, error) {
		localDigest, localExists, err = s.local.ResolveManifest(ctx, fullRepository, reference)
		if err != nil {
			return nil, err
		}
		if localExists && (localDigest == remoteDigest || strings.Contains(reference, ":")) {
			return nil, nil
		}
		token, err := s.tokens.IssueInternal("grom-pull-through", []Access{{
			Type: "repository", Name: fullRepository, Actions: []string{constants.RegistryActionPull, constants.RegistryActionPush},
		}})
		if err != nil {
			return nil, err
		}
		copyCtx, cancel := context.WithTimeout(ctx, s.timeout)
		defer cancel()
		digest, err := s.upstream.Copy(copyCtx, upstream, sourceRepository, fullRepository, reference, token)
		if err != nil {
			if localExists {
				s.logger.WarnContext(ctx, "serve local manifest while upstream copy is unavailable", "repository", fullRepository, "reference", reference)
				return nil, nil
			}
			return nil, err
		}
		if s.observe != nil {
			if err := s.observe(copyCtx, fullRepository, reference, digest, "grom-pull-through"); err != nil {
				s.logger.ErrorContext(copyCtx, "observe pull-through manifest", "repository", fullRepository, "reference", reference, "error", err)
			}
		}
		return nil, nil
	})
	return err
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
