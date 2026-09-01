package bunstore

import (
	"context"
	"strings"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

// SearchTagNamesPage answers tag search from the last-reconciled inventory
// snapshot (registry_tags), not a live registry call: substring search over
// every tag would otherwise require resolving each one against the upstream
// registry per request. Callers that need a live, dangling-tag-safe listing
// without a search term keep using ListLiveTagsPage.
func (s *Store) SearchTagNamesPage(ctx context.Context, repositoryID foundation.ID, search string, request foundation.PageRequest) (foundation.PageResult[string], error) {
	name := ""
	if request.Cursor != "" {
		cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
		if err != nil {
			return foundation.PageResult[string]{}, err
		}
		name = cursor.Name
	}
	var models []tagModel
	query := s.db.NewSelect().Model(&models).
		Where("repository_id = ?", repositoryID.String()).
		Where("detached_at IS NULL")
	if value := strings.ToLower(strings.TrimSpace(search)); value != "" {
		query = query.Where("LOWER(name) LIKE ? ESCAPE '\\'", likeContains(value))
	}
	if name != "" {
		query = query.Where("name > ?", name)
	}
	if err := query.OrderExpr("name ASC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[string]{}, err
	}
	count := len(models)
	if count > request.Limit {
		count = request.Limit
	}
	result := foundation.PageResult[string]{Items: make([]string, 0, count)}
	for i := 0; i < count; i++ {
		result.Items = append(result.Items, models[i].Name)
	}
	if len(models) > request.Limit {
		cursor, _ := foundation.EncodePageCursor(foundation.PageCursor{Scope: request.Scope, Name: models[request.Limit-1].Name})
		result.NextCursor = cursor
	}
	return result, nil
}
