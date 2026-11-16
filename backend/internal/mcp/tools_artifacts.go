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

type ListTagsInput struct {
	Project    string `json:"project" jsonschema:"project slug"`
	Repository string `json:"repository" jsonschema:"repository name inside the project"`
	Q          string `json:"q,omitempty" jsonschema:"case-insensitive tag substring"`
	Sort       string `json:"sort,omitempty" jsonschema:"newest (default), oldest, name-asc or name-desc"`
	Page
}

type ListArtifactsInput struct {
	Project    string `json:"project" jsonschema:"project slug"`
	Repository string `json:"repository" jsonschema:"repository name inside the project"`
	Q          string `json:"q,omitempty" jsonschema:"case-insensitive digest or live tag substring"`
	Page
}

type ArtifactInput struct {
	Project    string `json:"project" jsonschema:"project slug"`
	Repository string `json:"repository" jsonschema:"repository name inside the project"`
	Digest     string `json:"digest" jsonschema:"manifest digest, sha256:<hex>"`
}

type ListReferrersInput struct {
	Project    string `json:"project" jsonschema:"project slug"`
	Repository string `json:"repository" jsonschema:"repository name inside the project"`
	Digest     string `json:"digest" jsonschema:"subject manifest digest, sha256:<hex>"`
	Page
}

type ReferrersResult struct {
	SubjectDigest string     `json:"subjectDigest"`
	Items         []Artifact `json:"items"`
	NextCursor    string     `json:"nextCursor,omitempty" jsonschema:"pass as cursor with the same arguments to read the next page"`
}

func (t *tools) listTags(ctx context.Context, principal foundation.PrincipalRef, input ListTagsInput) (ListResult[string], error) {
	project, repository, err := t.visibleRepository(ctx, principal, input.Project, input.Repository)
	if err != nil {
		return ListResult[string]{}, err
	}
	query, err := normalizedQuery(input.Q)
	if err != nil {
		return ListResult[string]{}, err
	}
	sort, err := tagSort(input.Sort)
	if err != nil {
		return ListResult[string]{}, err
	}
	request, err := input.request("list_tags:" + project.Slug + ":" + repository.Name + ":q=" + strings.ToLower(query) + ":sort=" + string(sort))
	if err != nil {
		return ListResult[string]{}, err
	}
	page, err := t.Repositories.SearchTagNames(ctx, repository.ID, query, sort, request)
	if err != nil {
		return ListResult[string]{}, t.internal(ctx, "list tags", err)
	}
	return ListResult[string]{Items: nonNil(page.Items), NextCursor: page.NextCursor}, nil
}

func (t *tools) listArtifacts(ctx context.Context, principal foundation.PrincipalRef, input ListArtifactsInput) (ListResult[Artifact], error) {
	project, repository, err := t.inventoryRepository(ctx, principal, input.Project, input.Repository)
	if err != nil {
		return ListResult[Artifact]{}, err
	}
	query, err := normalizedQuery(input.Q)
	if err != nil {
		return ListResult[Artifact]{}, err
	}
	request, err := input.request("list_artifacts:" + project.Slug + ":" + repository + ":q=" + strings.ToLower(query))
	if err != nil {
		return ListResult[Artifact]{}, err
	}
	page, err := t.Inventory.ListPage(ctx, project.ID, repository, query, request)
	if err != nil {
		return ListResult[Artifact]{}, t.internal(ctx, "list artifacts", err)
	}
	return ListResult[Artifact]{Items: mapItems(page.Items, artifactDTO), NextCursor: page.NextCursor}, nil
}

func (t *tools) getArtifact(ctx context.Context, principal foundation.PrincipalRef, input ArtifactInput) (Artifact, error) {
	project, repository, err := t.inventoryRepository(ctx, principal, input.Project, input.Repository)
	if err != nil {
		return Artifact{}, err
	}
	if err := validDigest(input.Digest); err != nil {
		return Artifact{}, err
	}
	item, err := t.Inventory.Find(ctx, project.ID, repository, input.Digest)
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, errArtifactNotFound
	}
	if err != nil {
		return Artifact{}, t.internal(ctx, "find artifact", err)
	}
	return artifactDTO(*item), nil
}

func (t *tools) listReferrers(ctx context.Context, principal foundation.PrincipalRef, input ListReferrersInput) (ReferrersResult, error) {
	project, repository, err := t.inventoryRepository(ctx, principal, input.Project, input.Repository)
	if err != nil {
		return ReferrersResult{}, err
	}
	if err := validDigest(input.Digest); err != nil {
		return ReferrersResult{}, err
	}
	request, err := input.request("list_referrers:" + project.Slug + ":" + repository + ":" + input.Digest)
	if err != nil {
		return ReferrersResult{}, err
	}
	page, err := t.Inventory.ListReferrersPage(ctx, project.ID, repository, input.Digest, request)
	if err != nil {
		return ReferrersResult{}, t.internal(ctx, "list referrers", err)
	}
	return ReferrersResult{SubjectDigest: input.Digest, Items: mapItems(page.Items, artifactDTO), NextCursor: page.NextCursor}, nil
}

func (t *tools) listPolicyPresets(_ context.Context, _ foundation.PrincipalRef, _ struct{}) (ListResult[PolicyPreset], error) {
	if t.PolicyPresets == nil {
		return ListResult[PolicyPreset]{Items: []PolicyPreset{}}, nil
	}
	return ListResult[PolicyPreset]{Items: mapItems(t.PolicyPresets(), presetDTO)}, nil
}

// inventoryRepository authorizes a repository for inventory reads and
// returns its canonical name, which the inventory service resolves again
// inside the same project.
func (t *tools) inventoryRepository(ctx context.Context, principal foundation.PrincipalRef, slug, name string) (*projectdomain.Project, string, error) {
	project, repository, err := t.visibleRepository(ctx, principal, slug, name)
	if err != nil {
		return nil, "", err
	}
	if t.Inventory == nil {
		return nil, "", errUnavailable
	}
	return project, repository.Name, nil
}

func tagSort(raw string) (registrydomain.TagSort, error) {
	switch registrydomain.TagSort(raw) {
	case "":
		return registrydomain.TagSortNewest, nil
	case registrydomain.TagSortNewest, registrydomain.TagSortOldest, registrydomain.TagSortNameAsc, registrydomain.TagSortNameDesc:
		return registrydomain.TagSort(raw), nil
	default:
		return "", errInvalidSort
	}
}
