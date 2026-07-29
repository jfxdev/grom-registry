package domain

import (
	"context"
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
}
