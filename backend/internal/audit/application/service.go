package application

import (
	"context"
	"encoding/json"
	"time"

	auditdomain "github.com/jfxdev/grom/backend/internal/audit/domain"
	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Service struct {
	store auditdomain.Store
}

func New(store auditdomain.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Record(
	ctx context.Context,
	actor foundation.PrincipalRef,
	action, resourceKind string,
	resourceID foundation.ID,
	metadata map[string]any,
) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return s.store.Record(ctx, &auditdomain.Event{
		ID: foundation.NewID(), ActorKind: actor.Kind, ActorID: actor.ID,
		Action: action, ResourceKind: resourceKind, ResourceID: resourceID,
		MetadataJSON: string(raw), CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) RecordOnce(
	ctx context.Context,
	id foundation.ID,
	actor foundation.PrincipalRef,
	action, resourceKind string,
	resourceID foundation.ID,
	metadata map[string]any,
) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return s.store.RecordOnce(ctx, &auditdomain.Event{
		ID: id, ActorKind: actor.Kind, ActorID: actor.ID,
		Action: action, ResourceKind: resourceKind, ResourceID: resourceID,
		MetadataJSON: string(raw), CreatedAt: time.Now().UTC(),
	})
}
