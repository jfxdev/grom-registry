package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx,
			`ALTER TABLE registry_repositories
			 ADD COLUMN creation_source TEXT NOT NULL DEFAULT 'manual'`,
		)
		return err
	}, func(context.Context, *bun.DB) error {
		// SQLite cannot portably drop a column on every supported version.
		return nil
	})
}
