package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

var ErrProjectNotEmpty = errors.New("project contains repositories")

type ProjectDeletionGuard interface {
	ProjectHasRepositories(ctx context.Context, projectID foundation.ID) (bool, error)
}

type Service struct {
	repository    projectdomain.Repository
	deletionGuard ProjectDeletionGuard
}

func New(repository projectdomain.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) SetDeletionGuard(guard ProjectDeletionGuard) {
	s.deletionGuard = guard
}

func (s *Service) Create(
	ctx context.Context,
	actor foundation.PrincipalRef,
	systemAdmin bool,
	name, slug string,
) (*projectdomain.Project, error) {
	if !systemAdmin {
		return nil, errors.New("administrator permission required")
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	name = strings.TrimSpace(name)
	if name == "" || !slugPattern.MatchString(slug) {
		return nil, fmt.Errorf("project name and a valid lowercase slug are required")
	}
	project := &projectdomain.Project{
		ID: foundation.NewID(), Slug: slug, Name: name, CreatedBy: actor.ID, CreatedAt: time.Now().UTC(), AccountedUsage: foundation.PendingStorageUsage(),
	}
	membership := &projectdomain.Membership{
		ProjectID: project.ID, PrincipalKind: actor.Kind, PrincipalID: actor.ID,
		Role: constants.RoleAdmin, CreatedAt: time.Now().UTC(),
	}
	if err := s.repository.CreateProject(ctx, project, membership); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *Service) List(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool) ([]projectdomain.Project, error) {
	return s.repository.ListProjectsForPrincipal(ctx, actor, systemAdmin)
}

func (s *Service) ListPage(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, request foundation.PageRequest) (foundation.PageResult[projectdomain.Project], error) {
	paged, ok := s.repository.(projectdomain.PagedRepository)
	if !ok {
		return foundation.PageResult[projectdomain.Project]{}, errors.New("project pagination is not configured")
	}
	return paged.ListProjectsPageForPrincipal(ctx, actor, systemAdmin, request)
}

func (s *Service) Find(ctx context.Context, slug string) (*projectdomain.Project, error) {
	return s.repository.FindProjectBySlug(ctx, slug)
}

func (s *Service) Delete(ctx context.Context, systemAdmin bool, slug string) error {
	if !systemAdmin {
		return errors.New("administrator permission required")
	}
	project, err := s.repository.FindProjectBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if s.deletionGuard == nil {
		return errors.New("project deletion guard is not configured")
	}
	hasRepositories, err := s.deletionGuard.ProjectHasRepositories(ctx, project.ID)
	if err != nil {
		return err
	}
	if hasRepositories {
		return ErrProjectNotEmpty
	}
	return s.repository.DeleteProject(ctx, project.ID)
}

func (s *Service) ListMemberships(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, slug string) ([]projectdomain.Membership, error) {
	project, err := s.repository.FindProjectBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !systemAdmin {
		membership, findErr := s.repository.FindMembership(ctx, project.ID, actor)
		if findErr != nil || membership.Role != constants.RoleAdmin {
			return nil, errors.New("project administrator permission required")
		}
	}
	return s.repository.ListMemberships(ctx, project.ID)
}

func (s *Service) ListMembershipsPage(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, slug string, filter *projectdomain.MembershipPrincipalFilter, request foundation.PageRequest) (foundation.PageResult[projectdomain.Membership], error) {
	project, err := s.repository.FindProjectBySlug(ctx, slug)
	if err != nil {
		return foundation.PageResult[projectdomain.Membership]{}, err
	}
	if !systemAdmin {
		membership, findErr := s.repository.FindMembership(ctx, project.ID, actor)
		if findErr != nil || membership.Role != constants.RoleAdmin {
			return foundation.PageResult[projectdomain.Membership]{}, errors.New("project administrator permission required")
		}
	}
	paged, ok := s.repository.(projectdomain.PagedRepository)
	if !ok {
		return foundation.PageResult[projectdomain.Membership]{}, errors.New("project pagination is not configured")
	}
	return paged.ListMembershipsPage(ctx, project.ID, filter, request)
}

func (s *Service) SetMembership(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, slug string, principal foundation.PrincipalRef, role string) error {
	if role != constants.RoleReader && role != constants.RoleWriter && role != constants.RoleAdmin {
		return fmt.Errorf("invalid project role")
	}
	project, err := s.repository.FindProjectBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if !systemAdmin {
		membership, findErr := s.repository.FindMembership(ctx, project.ID, actor)
		if findErr != nil || membership.Role != constants.RoleAdmin {
			return errors.New("project administrator permission required")
		}
	}
	return s.repository.UpsertMembership(ctx, &projectdomain.Membership{
		ProjectID: project.ID, PrincipalKind: principal.Kind, PrincipalID: principal.ID,
		Role: role, CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) DeleteMembership(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, slug string, principal foundation.PrincipalRef) error {
	project, err := s.repository.FindProjectBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if !systemAdmin {
		membership, findErr := s.repository.FindMembership(ctx, project.ID, actor)
		if findErr != nil || membership.Role != constants.RoleAdmin {
			return errors.New("project administrator permission required")
		}
	}
	return s.repository.DeleteMembership(ctx, project.ID, principal)
}

func (s *Service) AllowedActions(ctx context.Context, principal foundation.PrincipalRef, repositoryName string, requested []string) []string {
	projectSlug := strings.SplitN(repositoryName, "/", 2)[0]
	if projectSlug == "" || projectSlug == repositoryName {
		return nil
	}
	project, err := s.repository.FindProjectBySlug(ctx, projectSlug)
	if err != nil {
		return nil
	}
	membership, err := s.repository.FindMembership(ctx, project.ID, principal)
	if err != nil {
		return nil
	}
	allowed := make([]string, 0, len(requested))
	for _, action := range requested {
		switch action {
		case constants.RegistryActionPull:
			allowed = append(allowed, action)
		case constants.RegistryActionPush:
			if membership.Role == constants.RoleWriter || membership.Role == constants.RoleAdmin {
				allowed = append(allowed, action)
			}
		}
	}
	return allowed
}

func (s *Service) CanView(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, project *projectdomain.Project) bool {
	if systemAdmin {
		return true
	}
	_, err := s.repository.FindMembership(ctx, project.ID, actor)
	return !errors.Is(err, sql.ErrNoRows) && err == nil
}

func (s *Service) CanManage(ctx context.Context, actor foundation.PrincipalRef, systemAdmin bool, project *projectdomain.Project) bool {
	if systemAdmin {
		return true
	}
	membership, err := s.repository.FindMembership(ctx, project.ID, actor)
	return err == nil && membership.Role == constants.RoleAdmin
}
