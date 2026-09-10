package bunstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

// SearchTagNamesPage answers tag listing/search from the last-reconciled
// inventory snapshot (registry_tags), not a live registry call: substring
// search and non-name sort orders would otherwise require resolving every
// tag against the upstream registry per request.
func (s *Store) SearchTagNamesPage(ctx context.Context, repositoryID foundation.ID, search string, sort registrydomain.TagSort, request foundation.PageRequest) (foundation.PageResult[string], error) {
	var cursorName, cursorTimestamp string
	if request.Cursor != "" {
		cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
		if err != nil {
			return foundation.PageResult[string]{}, err
		}
		cursorName = cursor.Name
		cursorTimestamp = cursor.Timestamp
	}

	var models []tagModel
	query := s.db.NewSelect().Model(&models).
		Where("repository_id = ?", repositoryID.String()).
		Where("detached_at IS NULL")
	if value := strings.ToLower(strings.TrimSpace(search)); value != "" {
		query = query.Where("LOWER(name) LIKE ? ESCAPE '\\'", likeContains(value))
	}

	orderExpr, err := tagSortOrderExpr(sort)
	if err != nil {
		return foundation.PageResult[string]{}, err
	}
	if cursorName != "" {
		predicate, args, predicateErr := tagSortKeysetPredicate(sort, cursorName, cursorTimestamp)
		if predicateErr != nil {
			return foundation.PageResult[string]{}, predicateErr
		}
		query = query.Where(predicate, args...)
	}

	if err := query.OrderExpr(orderExpr).Limit(request.Limit + 1).Scan(ctx); err != nil {
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
		last := models[request.Limit-1]
		cursor := foundation.PageCursor{Scope: request.Scope, Name: last.Name}
		if tagSortUsesTimestamp(sort) {
			cursor.Timestamp = last.LastMovedAt.UTC().Format(time.RFC3339Nano)
		}
		encoded, _ := foundation.EncodePageCursor(cursor)
		result.NextCursor = encoded
	}
	return result, nil
}

func tagSortUsesTimestamp(sort registrydomain.TagSort) bool {
	return sort == registrydomain.TagSortNewest || sort == registrydomain.TagSortOldest
}

func tagSortOrderExpr(sort registrydomain.TagSort) (string, error) {
	switch sort {
	case "", registrydomain.TagSortNewest:
		return "last_moved_at DESC, name ASC", nil
	case registrydomain.TagSortOldest:
		return "last_moved_at ASC, name ASC", nil
	case registrydomain.TagSortNameAsc:
		return "name ASC", nil
	case registrydomain.TagSortNameDesc:
		return "name DESC", nil
	default:
		return "", fmt.Errorf("unsupported tag sort %q", sort)
	}
}

func tagSortKeysetPredicate(sort registrydomain.TagSort, name, timestamp string) (string, []any, error) {
	switch sort {
	case registrydomain.TagSortNameDesc:
		return "name < ?", []any{name}, nil
	case registrydomain.TagSortNameAsc:
		return "name > ?", []any{name}, nil
	case "", registrydomain.TagSortNewest, registrydomain.TagSortOldest:
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return "", nil, fmt.Errorf("invalid cursor timestamp: %w", err)
		}
		comparator := "<"
		if sort == registrydomain.TagSortOldest {
			comparator = ">"
		}
		predicate := fmt.Sprintf("(last_moved_at %s ?) OR (last_moved_at = ? AND name > ?)", comparator)
		return predicate, []any{parsed, parsed, name}, nil
	default:
		return "", nil, fmt.Errorf("unsupported tag sort %q", sort)
	}
}
