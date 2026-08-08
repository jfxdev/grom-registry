package httpapi

import (
	"encoding/base64"
	"encoding/json"
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

// pageCursor is deliberately opaque to clients. Its scope prevents a cursor
// created for one list or filter from being replayed against another one.
type pageCursor struct {
	Version int    `json:"v"`
	Scope   string `json:"s"`
	Offset  int    `json:"o"`
}

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
		return foundation.PageRequest{Limit: limit}, 0, nil
	}
	if len(cursor) > maxPageCursorLen {
		return foundation.PageRequest{}, 0, fmt.Errorf("invalid cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return foundation.PageRequest{}, 0, fmt.Errorf("invalid cursor")
	}
	var decoded pageCursor
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Version != 1 || decoded.Scope != scope || decoded.Offset < 0 {
		return foundation.PageRequest{}, 0, fmt.Errorf("invalid cursor")
	}
	return foundation.PageRequest{Cursor: cursor, Limit: limit}, decoded.Offset, nil
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
	pageCount := (len(values) + request.Limit - 1) / request.Limit
	result := foundation.PageResult[T]{Items: values[offset:end], PageCount: pageCount}
	if end < len(values) {
		nextRaw, _ := json.Marshal(pageCursor{Version: 1, Scope: scope, Offset: end})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(nextRaw)
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
