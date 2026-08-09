package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

const (
	defaultPageLimit = 25
	maxPageLimit     = 100
	maxPageCursorLen = 512
)

func pageRequest(r *http.Request, scope string) (foundation.PageRequest, int, error) {
	limit := defaultPageLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxPageLimit {
			return foundation.PageRequest{}, 0, fmt.Errorf("invalid limit")
		}
		limit = parsed
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		return foundation.PageRequest{Limit: limit, Scope: scope}, 0, nil
	}
	if len(cursor) > maxPageCursorLen {
		return foundation.PageRequest{}, 0, fmt.Errorf("invalid cursor")
	}
	decoded, err := foundation.DecodePageCursor(cursor, scope)
	if err != nil {
		return foundation.PageRequest{}, 0, fmt.Errorf("invalid cursor")
	}
	return foundation.PageRequest{Cursor: cursor, Limit: limit, Scope: scope}, decoded.Offset, nil
}

// pageResult paginates an already materialized slice. Its offset cursor is
// stable for an unchanged list, but it is not a datastore keyset cursor: list
// mutations can shift subsequent pages. List endpoints should move to
// datastore-backed keyset pagination before their result sets become large.
func pageResult[T any](r *http.Request, scope string, values []T) (foundation.PageResult[T], error) {
	request, offset, err := pageRequest(r, scope)
	if err != nil {
		return foundation.PageResult[T]{}, err
	}
	if offset > len(values) {
		return foundation.PageResult[T]{}, fmt.Errorf("invalid cursor")
	}
	end := offset + request.Limit
	if end > len(values) {
		end = len(values)
	}
	result := foundation.PageResult[T]{Items: values[offset:end]}
	if end < len(values) {
		result.NextCursor, _ = foundation.EncodePageCursor(foundation.PageCursor{Scope: scope, Offset: end})
	}
	return result, nil
}

func writePage[T any](w http.ResponseWriter, r *http.Request, scope string, values []T) {
	result, err := pageResult(r, scope, values)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
