package bunstore

import (
	"context"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type repositorySearchModel struct {
	ID          string    `bun:"id"`
	ProjectID   string    `bun:"project_id"`
	ProjectSlug string    `bun:"project_slug"`
	ProjectName string    `bun:"project_name"`
	Name        string    `bun:"name"`
	Description string    `bun:"description"`
	Status      string    `bun:"status"`
	CreatedAt   time.Time `bun:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at"`
}

// SearchRepositoriesAcrossProjects powers installation-administrator search:
// unlike ListRepositoriesPage it is not scoped to a single project, so the
// keyset must break ties on project_id (repository names are only unique
// within a project).
func (s *Store) SearchRepositoriesAcrossProjects(ctx context.Context, search string, request foundation.PageRequest) (foundation.PageResult[registrydomain.RepositorySearchResult], error) {
	name, projectID := "", ""
	if request.Cursor != "" {
		cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
		if err != nil {
			return foundation.PageResult[registrydomain.RepositorySearchResult]{}, err
		}
		name, projectID = cursor.Name, cursor.ID
	}
	models := make([]repositorySearchModel, 0, request.Limit+1)
	query := s.db.NewSelect().
		ColumnExpr("rr.id AS id, rr.project_id AS project_id, p.slug AS project_slug, p.name AS project_name, rr.name AS name, rr.description AS description, rr.status AS status, rr.created_at AS created_at, rr.updated_at AS updated_at").
		TableExpr("registry_repositories AS rr").
		Join("JOIN projects AS p ON p.id = rr.project_id")
	if value := strings.ToLower(strings.TrimSpace(search)); value != "" {
		like := likeContains(value)
		query = query.Where("(LOWER(rr.name) LIKE ? ESCAPE '\\' OR LOWER(rr.description) LIKE ? ESCAPE '\\')", like, like)
	}
	if name != "" {
		query = query.Where("(rr.name > ? OR (rr.name = ? AND rr.project_id > ?))", name, name, projectID)
	}
	if err := query.OrderExpr("rr.name ASC, rr.project_id ASC").Limit(request.Limit+1).Scan(ctx, &models); err != nil {
		return foundation.PageResult[registrydomain.RepositorySearchResult]{}, err
	}
	count := len(models)
	if count > request.Limit {
		count = request.Limit
	}
	result := foundation.PageResult[registrydomain.RepositorySearchResult]{Items: make([]registrydomain.RepositorySearchResult, 0, count)}
	for i := 0; i < count; i++ {
		model := models[i]
		result.Items = append(result.Items, registrydomain.RepositorySearchResult{
			ID:          foundation.ID(model.ID),
			ProjectID:   foundation.ID(model.ProjectID),
			ProjectSlug: model.ProjectSlug,
			ProjectName: model.ProjectName,
			Name:        model.Name,
			Description: model.Description,
			Status:      model.Status,
			CreatedAt:   model.CreatedAt,
			UpdatedAt:   model.UpdatedAt,
		})
	}
	if len(models) > request.Limit {
		last := models[request.Limit-1]
		cursor, _ := foundation.EncodePageCursor(foundation.PageCursor{Scope: request.Scope, Name: last.Name, ID: last.ProjectID})
		result.NextCursor = cursor
	}
	return result, nil
}
