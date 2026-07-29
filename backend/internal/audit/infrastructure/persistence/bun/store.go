package bunstore

import (
	"context"
	"time"

	auditdomain "github.com/jfxdev/grom/backend/internal/audit/domain"
	"github.com/uptrace/bun"
)

type Store struct {
	db *bun.DB
}

type eventModel struct {
	bun.BaseModel `bun:"table:audit_events,alias:ae"`
	ID            string    `bun:"id,pk"`
	ActorKind     string    `bun:"actor_kind,notnull"`
	ActorID       string    `bun:"actor_id,notnull"`
	Action        string    `bun:"action,notnull"`
	ResourceKind  string    `bun:"resource_kind,notnull"`
	ResourceID    string    `bun:"resource_id,notnull"`
	MetadataJSON  string    `bun:"metadata_json,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull"`
}

func New(db *bun.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Record(ctx context.Context, event *auditdomain.Event) error {
	_, err := s.db.NewInsert().Model(&eventModel{
		ID: event.ID.String(), ActorKind: event.ActorKind, ActorID: event.ActorID.String(),
		Action: event.Action, ResourceKind: event.ResourceKind, ResourceID: event.ResourceID.String(),
		MetadataJSON: event.MetadataJSON, CreatedAt: event.CreatedAt,
	}).Exec(ctx)
	return err
}

var _ auditdomain.Store = (*Store)(nil)
