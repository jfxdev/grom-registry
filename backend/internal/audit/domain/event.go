package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Event struct {
	ID           foundation.ID
	ActorKind    string
	ActorID      foundation.ID
	Action       string
	ResourceKind string
	ResourceID   foundation.ID
	MetadataJSON string
	CreatedAt    time.Time
}

type Store interface {
	Record(ctx context.Context, event *Event) error
	RecordOnce(ctx context.Context, event *Event) error
}

// Filter narrows an audit-event listing. Zero-value fields are ignored.
type Filter struct {
	Actor        string    // case-insensitive substring across resolved actor name/username
	Action       string    // exact match
	ResourceKind string    // exact match
	From         time.Time // inclusive lower bound on CreatedAt
	To           time.Time // exclusive upper bound on CreatedAt
}

// ListItem is a read projection of an audit event with the actor resolved to a
// display name and username. Its JSON tags mirror the AuditEvent OpenAPI schema
// because the HTTP layer serializes it directly. Metadata is already sanitized
// at write time, so it is exposed verbatim.
type ListItem struct {
	ID            foundation.ID   `json:"id"`
	ActorKind     string          `json:"actorKind"`
	ActorID       foundation.ID   `json:"actorId"`
	ActorName     string          `json:"actorName,omitempty"`
	ActorUsername string          `json:"actorUsername,omitempty"`
	Action        string          `json:"action"`
	ResourceKind  string          `json:"resourceKind"`
	ResourceID    foundation.ID   `json:"resourceId"`
	Metadata      json.RawMessage `json:"metadata"`
	CreatedAt     time.Time       `json:"createdAt"`
}

// Reader is the read port for audit events. It is intentionally separate from
// Store so the write side stays append-only and events remain immutable.
type Reader interface {
	List(ctx context.Context, filter Filter, request foundation.PageRequest) (foundation.PageResult[ListItem], error)
}
