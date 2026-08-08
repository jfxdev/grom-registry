package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, "ALTER TABLE password_reset_tokens ADD COLUMN purpose TEXT NOT NULL DEFAULT 'password_reset'")
		return err
	}, func(context.Context, *bun.DB) error {
		// SQLite cannot portably drop a column on every supported version.
		return nil
	})
}
