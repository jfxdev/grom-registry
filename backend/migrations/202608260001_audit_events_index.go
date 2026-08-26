package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(addAuditEventsIndex, dropAuditEventsIndex)
}

// addAuditEventsIndex backs keyset pagination and time-range filtering of the
// audit-event listing endpoint. The table previously had only its primary key.
func addAuditEventsIndex(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx,
		`CREATE INDEX idx_audit_events_created_at_id ON audit_events (created_at, id)`)
	return err
}

func dropAuditEventsIndex(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_audit_events_created_at_id`)
	return err
}
