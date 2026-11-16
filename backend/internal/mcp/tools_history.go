package mcp

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
)

type HistoryInput struct {
	Project    string `json:"project" jsonschema:"project slug"`
	Repository string `json:"repository" jsonschema:"repository name inside the project"`
	Page
}

type LifecycleRunInput struct {
	Project string `json:"project" jsonschema:"project slug"`
	RunID   string `json:"runId" jsonschema:"lifecycle run id returned by list_lifecycle_runs"`
}

// History matches the web API: lifecycle and deletion history is visible only
// to project administrators. A member without Admin receives the same
// not-found result as a non-member.

func (t *tools) listLifecycleRuns(ctx context.Context, principal foundation.PrincipalRef, input HistoryInput) (ListResult[LifecycleRun], error) {
	project, repository, err := t.historyRepository(ctx, principal, input.Project, input.Repository, t.Lifecycle != nil)
	if err != nil {
		return ListResult[LifecycleRun]{}, err
	}
	request, err := input.request("list_lifecycle_runs:" + project.Slug + ":" + repository)
	if err != nil {
		return ListResult[LifecycleRun]{}, err
	}
	page, err := t.Lifecycle.ListRunsPage(ctx, project.ID, repository, request)
	if err != nil {
		return ListResult[LifecycleRun]{}, t.internal(ctx, "list lifecycle runs", err)
	}
	return ListResult[LifecycleRun]{Items: mapItems(page.Items, lifecycleRunDTO), NextCursor: page.NextCursor}, nil
}

func (t *tools) getLifecycleRun(ctx context.Context, principal foundation.PrincipalRef, input LifecycleRunInput) (LifecycleRun, error) {
	project, err := t.manageableProject(ctx, principal, input.Project)
	if err != nil {
		return LifecycleRun{}, err
	}
	if err := validRunID(input.RunID); err != nil {
		return LifecycleRun{}, err
	}
	if t.Lifecycle == nil {
		return LifecycleRun{}, errUnavailable
	}
	run, err := t.Lifecycle.FindRun(ctx, foundation.ID(input.RunID))
	if errors.Is(err, sql.ErrNoRows) {
		return LifecycleRun{}, errRunNotFound
	}
	if err != nil {
		return LifecycleRun{}, t.internal(ctx, "find lifecycle run", err)
	}
	repository, err := t.Repositories.FindByID(ctx, run.RepositoryID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && repository.ProjectID != project.ID) {
		return LifecycleRun{}, errRunNotFound
	}
	if err != nil {
		return LifecycleRun{}, t.internal(ctx, "find lifecycle run repository", err)
	}
	run.Repository = repository.Name
	return lifecycleRunDTO(*run), nil
}

func (t *tools) listArtifactDeletions(ctx context.Context, principal foundation.PrincipalRef, input HistoryInput) (ListResult[ArtifactDeletion], error) {
	project, repository, err := t.historyRepository(ctx, principal, input.Project, input.Repository, t.Deletions != nil)
	if err != nil {
		return ListResult[ArtifactDeletion]{}, err
	}
	request, err := input.request("list_artifact_deletions:" + project.Slug + ":" + repository)
	if err != nil {
		return ListResult[ArtifactDeletion]{}, err
	}
	page, err := t.Deletions.ListPage(ctx, project.ID, repository, request)
	if err != nil {
		return ListResult[ArtifactDeletion]{}, t.internal(ctx, "list artifact deletions", err)
	}
	return ListResult[ArtifactDeletion]{Items: mapItems(page.Items, artifactDeletionDTO), NextCursor: page.NextCursor}, nil
}

func (t *tools) historyRepository(ctx context.Context, principal foundation.PrincipalRef, slug, name string, available bool) (*projectdomain.Project, string, error) {
	project, err := t.manageableProject(ctx, principal, slug)
	if err != nil {
		return nil, "", err
	}
	if err := validRepository(name); err != nil {
		return nil, "", err
	}
	repository, err := t.Repositories.Find(ctx, project.ID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", errRepositoryNotFound
	}
	if err != nil {
		return nil, "", t.internal(ctx, "find repository", err)
	}
	if !available {
		return nil, "", errUnavailable
	}
	return project, repository.Name, nil
}
