package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(
			ctx,
			"ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE",
		)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, "ALTER TABLE users DROP COLUMN must_change_password")
		return err
	})
}
