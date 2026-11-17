package mcp

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type ListProjectsInput struct {
	Page
}

type ProjectInput struct {
	Project string `json:"project" jsonschema:"project slug"`
}

type ListRepositoriesInput struct {
	Project string `json:"project" jsonschema:"project slug"`
	Q       string `json:"q,omitempty" jsonschema:"case-insensitive repository name substring"`
	Page
}

type RepositoryInput struct {
	Project    string `json:"project" jsonschema:"project slug"`
	Repository string `json:"repository" jsonschema:"repository name inside the project, without the project slug"`
}

// Access-key principals are never installation administrators: visibility
// always comes from explicit project membership.
const systemAdmin = false

func (t *tools) listProjects(ctx context.Context, principal foundation.PrincipalRef, input ListProjectsInput) (ListResult[Project], error) {
	request, err := input.request("list_projects:" + principal.Kind + ":" + principal.ID.String())
	if err != nil {
		return ListResult[Project]{}, err
	}
	page, err := t.Projects.ListPage(ctx, principal, systemAdmin, request)
	if err != nil {
		return ListResult[Project]{}, t.internal(ctx, "list projects", err)
	}
	ids := make([]foundation.ID, len(page.Items))
	for i, project := range page.Items {
		ids[i] = project.ID
	}
	usages := t.projectUsages(ctx, ids)
	for i := range page.Items {
		page.Items[i].AccountedUsage = usages[page.Items[i].ID]
	}
	return ListResult[Project]{Items: mapItems(page.Items, projectDTO), NextCursor: page.NextCursor}, nil
}

func (t *tools) getProject(ctx context.Context, principal foundation.PrincipalRef, input ProjectInput) (ProjectDetail, error) {
	project, err := t.visibleProject(ctx, principal, input.Project)
	if err != nil {
		return ProjectDetail{}, err
	}
	project.AccountedUsage = t.projectUsage(ctx, project.ID)
	return ProjectDetail{
		Project:   projectDTO(*project),
		CanManage: t.Projects.CanManage(ctx, principal, systemAdmin, project),
	}, nil
}

func (t *tools) listRepositories(ctx context.Context, principal foundation.PrincipalRef, input ListRepositoriesInput) (ListResult[Repository], error) {
	project, err := t.visibleProject(ctx, principal, input.Project)
	if err != nil {
		return ListResult[Repository]{}, err
	}
	query, err := normalizedQuery(input.Q)
	if err != nil {
		return ListResult[Repository]{}, err
	}
	request, err := input.request("list_repositories:" + project.Slug + ":q=" + strings.ToLower(query))
	if err != nil {
		return ListResult[Repository]{}, err
	}
	page, err := t.Repositories.ListPage(ctx, project.ID, query, request)
	if err != nil {
		return ListResult[Repository]{}, t.internal(ctx, "list repositories", err)
	}
	return ListResult[Repository]{Items: mapItems(page.Items, repositoryDTO), NextCursor: page.NextCursor}, nil
}

func (t *tools) getRepository(ctx context.Context, principal foundation.PrincipalRef, input RepositoryInput) (Repository, error) {
	_, repository, err := t.visibleRepository(ctx, principal, input.Project, input.Repository)
	if err != nil {
		return Repository{}, err
	}
	return repositoryDTO(*repository), nil
}

func (t *tools) visibleProject(ctx context.Context, principal foundation.PrincipalRef, slug string) (*projectdomain.Project, error) {
	return t.authorizedProject(ctx, principal, slug, false)
}

func (t *tools) manageableProject(ctx context.Context, principal foundation.PrincipalRef, slug string) (*projectdomain.Project, error) {
	return t.authorizedProject(ctx, principal, slug, true)
}

func (t *tools) authorizedProject(ctx context.Context, principal foundation.PrincipalRef, slug string, manage bool) (*projectdomain.Project, error) {
	if err := validProject(slug); err != nil {
		return nil, err
	}
	project, err := t.Projects.Find(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errProjectNotFound
	}
	if err != nil {
		return nil, t.internal(ctx, "find project", err)
	}
	allowed := t.Projects.CanView(ctx, principal, systemAdmin, project)
	if manage {
		allowed = t.Projects.CanManage(ctx, principal, systemAdmin, project)
	}
	if !allowed {
		return nil, errProjectNotFound
	}
	return project, nil
}

// visibleRepository resolves the repository inside the authorized project, so
// a repository can never be read through another project's slug.
func (t *tools) visibleRepository(ctx context.Context, principal foundation.PrincipalRef, slug, name string) (*projectdomain.Project, *registrydomain.Repository, error) {
	project, err := t.visibleProject(ctx, principal, slug)
	if err != nil {
		return nil, nil, err
	}
	if err := validRepository(name); err != nil {
		return nil, nil, err
	}
	repository, err := t.Repositories.Find(ctx, project.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errRepositoryNotFound
	}
	if err != nil {
		return nil, nil, t.internal(ctx, "find repository", err)
	}
	return project, repository, nil
}

func (t *tools) projectUsage(ctx context.Context, projectID foundation.ID) foundation.AccountedStorageUsage {
	if t.Inventory == nil {
		return foundation.AccountedStorageUsage{Status: "unavailable"}
	}
	return t.Inventory.ProjectUsage(ctx, projectID)
}

func (t *tools) projectUsages(ctx context.Context, projectIDs []foundation.ID) map[foundation.ID]foundation.AccountedStorageUsage {
	if t.Inventory == nil {
		result := make(map[foundation.ID]foundation.AccountedStorageUsage, len(projectIDs))
		for _, id := range projectIDs {
			result[id] = foundation.AccountedStorageUsage{Status: "unavailable"}
		}
		return result
	}
	return t.Inventory.ProjectUsages(ctx, projectIDs)
}

func (t *tools) internal(ctx context.Context, operation string, err error) error {
	t.Logger.ErrorContext(ctx, "mcp tool failed", "operation", operation, "error", err)
	return errUnavailable
}
