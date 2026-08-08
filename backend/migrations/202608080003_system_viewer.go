package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(addSystemViewerColumn, func(context.Context, *bun.DB) error {
		return nil
	})
}

func addSystemViewerColumn(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN is_system_viewer BOOLEAN NOT NULL DEFAULT FALSE")
	return err
}
