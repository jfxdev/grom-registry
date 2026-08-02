package application

import (
	"context"
	"encoding/json"
	"strings"
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
	cleaned, err := sanitizeMetadata(metadata)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cleaned)
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
	cleaned, err := sanitizeMetadata(metadata)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return err
	}
	return s.store.RecordOnce(ctx, &auditdomain.Event{
		ID: id, ActorKind: actor.Kind, ActorID: actor.ID,
		Action: action, ResourceKind: resourceKind, ResourceID: resourceID,
		MetadataJSON: string(raw), CreatedAt: time.Now().UTC(),
	})
}

func sanitizeMetadata(metadata map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return sanitizeNormalizedMetadata(normalized), nil
}

func sanitizeNormalizedMetadata(metadata map[string]any) map[string]any {
	cleaned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if sensitiveMetadataKey(key) {
			continue
		}
		cleaned[key] = sanitizeMetadataValue(value)
	}
	return cleaned
}

func sanitizeMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeNormalizedMetadata(typed)
	case []any:
		cleaned := make([]any, len(typed))
		for index, item := range typed {
			cleaned[index] = sanitizeMetadataValue(item)
		}
		return cleaned
	default:
		return value
	}
}

func sensitiveMetadataKey(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", "")) {
	case "password", "secret", "token", "accesskey", "tokensecret", "resettoken", "accesskeysecret", "authorization", "sessionsecret":
		return true
	default:
		return false
	}
}
