package bunstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	auditdomain "github.com/jfxdev/grom/backend/internal/audit/domain"
	"github.com/jfxdev/grom/backend/internal/foundation"
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
	return s.record(ctx, event, false)
}

func (s *Store) RecordOnce(ctx context.Context, event *auditdomain.Event) error {
	return s.record(ctx, event, true)
}

func (s *Store) record(ctx context.Context, event *auditdomain.Event, once bool) error {
	query := s.db.NewInsert().Model(&eventModel{
		ID: event.ID.String(), ActorKind: event.ActorKind, ActorID: event.ActorID.String(),
		Action: event.Action, ResourceKind: event.ResourceKind, ResourceID: event.ResourceID.String(),
		MetadataJSON: event.MetadataJSON, CreatedAt: event.CreatedAt,
	})
	if once {
		query = query.On("CONFLICT (id) DO NOTHING")
	}
	_, err := query.Exec(ctx)
	return err
}

var _ auditdomain.Store = (*Store)(nil)
var _ auditdomain.Reader = (*Store)(nil)

type auditKeyset struct {
	at time.Time
	id string
}

func auditBoundary(request foundation.PageRequest) (*auditKeyset, error) {
	if request.Cursor == "" {
		return nil, nil
	}
	cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
	if err != nil || cursor.Timestamp == "" || cursor.ID == "" {
		return nil, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, cursor.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &auditKeyset{at: at, id: cursor.ID}, nil
}

func auditNext(scope string, at time.Time, id string) string {
	cursor, _ := foundation.EncodePageCursor(foundation.PageCursor{Scope: scope, Timestamp: at.UTC().Format(time.RFC3339Nano), ID: id})
	return cursor
}

type listRow struct {
	ID           string    `bun:"id"`
	ActorKind    string    `bun:"actor_kind"`
	ActorID      string    `bun:"actor_id"`
	Action       string    `bun:"action"`
	ResourceKind string    `bun:"resource_kind"`
	ResourceID   string    `bun:"resource_id"`
	MetadataJSON string    `bun:"metadata_json"`
	CreatedAt    time.Time `bun:"created_at"`
	UserUsername string    `bun:"user_username"`
	SAName       string    `bun:"sa_name"`
	SAUsername   string    `bun:"sa_username"`
}

func (s *Store) List(ctx context.Context, filter auditdomain.Filter, request foundation.PageRequest) (foundation.PageResult[auditdomain.ListItem], error) {
	boundary, err := auditBoundary(request)
	if err != nil {
		return foundation.PageResult[auditdomain.ListItem]{}, err
	}

	rows := make([]listRow, 0, request.Limit+1)
	query := s.db.NewSelect().
		Model(&rows).
		ModelTableExpr("audit_events AS ae").
		ColumnExpr("ae.id, ae.actor_kind, ae.actor_id, ae.action, ae.resource_kind, ae.resource_id, ae.metadata_json, ae.created_at").
		ColumnExpr("u.username AS user_username").
		ColumnExpr("sa.name AS sa_name").
		ColumnExpr("sa.username AS sa_username").
		Join("LEFT JOIN users AS u ON u.id = ae.actor_id").
		Join("LEFT JOIN service_accounts AS sa ON sa.id = ae.actor_id")

	if filter.Action != "" {
		query = query.Where("ae.action = ?", filter.Action)
	}
	if filter.ResourceKind != "" {
		query = query.Where("ae.resource_kind = ?", filter.ResourceKind)
	}
	if !filter.From.IsZero() {
		query = query.Where("ae.created_at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		query = query.Where("ae.created_at < ?", filter.To)
	}
	if actor := strings.ToLower(strings.TrimSpace(filter.Actor)); actor != "" {
		like := "%" + escapeLike(actor) + "%"
		query = query.Where(
			"(LOWER(u.username) LIKE ? ESCAPE '\\' OR LOWER(sa.name) LIKE ? ESCAPE '\\' OR LOWER(sa.username) LIKE ? ESCAPE '\\' OR LOWER(ae.actor_id) LIKE ? ESCAPE '\\')",
			like, like, like, like,
		)
	}
	if boundary != nil {
		query = query.Where("(ae.created_at < ? OR (ae.created_at = ? AND ae.id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("ae.created_at DESC, ae.id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[auditdomain.ListItem]{}, err
	}

	result := foundation.PageResult[auditdomain.ListItem]{Items: make([]auditdomain.ListItem, 0, min(len(rows), request.Limit))}
	for _, row := range rows[:min(len(rows), request.Limit)] {
		result.Items = append(result.Items, toListItem(row))
	}
	if len(rows) > request.Limit {
		last := rows[request.Limit-1]
		result.NextCursor = auditNext(request.Scope, last.CreatedAt, last.ID)
	}
	return result, nil
}

func escapeLike(value string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
}

func toListItem(row listRow) auditdomain.ListItem {
	name, username := resolveActor(row)
	metadata := row.MetadataJSON
	if strings.TrimSpace(metadata) == "" {
		metadata = "{}"
	}
	return auditdomain.ListItem{
		ID:            foundation.ID(row.ID),
		ActorKind:     row.ActorKind,
		ActorID:       foundation.ID(row.ActorID),
		ActorName:     name,
		ActorUsername: username,
		Action:        row.Action,
		ResourceKind:  row.ResourceKind,
		ResourceID:    foundation.ID(row.ResourceID),
		Metadata:      json.RawMessage(metadata),
		CreatedAt:     row.CreatedAt,
	}
}

// resolveActor maps a joined row to a display name and username. actor_id is a
// UUID unique across users and service accounts, so at most one join matches.
func resolveActor(row listRow) (name string, username string) {
	switch {
	case row.UserUsername != "":
		return row.UserUsername, row.UserUsername
	case row.SAUsername != "":
		return row.SAName, row.SAUsername
	default:
		return "", ""
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
