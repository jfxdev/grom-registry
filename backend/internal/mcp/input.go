package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
)

// Tool errors carry fixed text only. Service errors are logged, never
// returned, and a hidden project is indistinguishable from a missing one.
var (
	errUnauthenticated    = errors.New("authentication required")
	errProjectNotFound    = errors.New("project not found")
	errRepositoryNotFound = errors.New("repository not found")
	errArtifactNotFound   = errors.New("artifact not found")
	errRunNotFound        = errors.New("lifecycle run not found")
	errUnavailable        = errors.New("registry metadata is temporarily unavailable")
	errInvalidProject     = errors.New("project must be a project slug")
	errInvalidRepository  = errors.New("repository must be a repository name inside the project")
	errInvalidDigest      = errors.New("digest must be sha256:<64 hex> or sha512:<128 hex>")
	errInvalidRunID       = errors.New("runId must be a lifecycle run UUID")
	errInvalidQuery       = errors.New("q is too long")
	errInvalidSort        = errors.New("sort must be newest, oldest, name-asc or name-desc")
	errInvalidPageRequest = errors.New("cursor or limit is invalid")
)

var (
	slugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	digestPattern = regexp.MustCompile(`^(?:sha256:[a-f0-9]{64}|sha512:[a-f0-9]{128})$`)
)

func validProject(slug string) error {
	if len(slug) == 0 || len(slug) > constants.MaxNameLength || !slugPattern.MatchString(slug) {
		return errInvalidProject
	}
	return nil
}

func validRepository(name string) error {
	if name == "" || len(name) > constants.MCPMaxRepositoryLen || strings.HasPrefix(name, "/") ||
		strings.HasSuffix(name, "/") || strings.Contains(name, "..") || strings.Contains(name, "//") {
		return errInvalidRepository
	}
	return nil
}

func validDigest(digest string) error {
	if !digestPattern.MatchString(digest) {
		return errInvalidDigest
	}
	return nil
}

func validRunID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return errInvalidRunID
	}
	return nil
}

func normalizedQuery(q string) (string, error) {
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) > constants.MCPMaxQueryRunes {
		return "", errInvalidQuery
	}
	return q, nil
}

// Page is the shared pagination input. Cursors are opaque and bound to the
// tool and arguments that produced them. The binding is a fixed-length digest
// of those arguments, so a cursor produced for the longest accepted arguments
// still fits the cursor length limit.
type Page struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"opaque nextCursor returned by the previous call with the same arguments"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum items to return, 1-100, default 25"`
}

func (p Page) request(scope string) (foundation.PageRequest, error) {
	limit := p.Limit
	if limit == 0 {
		limit = constants.MCPDefaultPageLimit
	}
	if limit < 1 || limit > constants.MCPMaxPageLimit || len(p.Cursor) > constants.MCPMaxCursorLength {
		return foundation.PageRequest{}, errInvalidPageRequest
	}
	digest := sha256.Sum256([]byte(scope))
	scope = "mcp:" + hex.EncodeToString(digest[:])
	if p.Cursor != "" {
		if _, err := foundation.DecodePageCursor(p.Cursor, scope); err != nil {
			return foundation.PageRequest{}, errInvalidPageRequest
		}
	}
	return foundation.PageRequest{Cursor: p.Cursor, Limit: limit, Scope: scope}, nil
}
