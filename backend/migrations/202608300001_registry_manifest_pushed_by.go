package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

func init() {
	Collection.MustRegister(ensureManifestLastPushedByColumn, preventManifestLastPushedByRollback)
}

func ensureManifestLastPushedByColumn(ctx context.Context, db *bun.DB) error {
	var count int
	switch db.Dialect().Name() {
	case dialect.PG:
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'registry_manifests'
			  AND column_name = 'last_pushed_by'
		`).Scan(ctx, &count); err != nil {
			return fmt.Errorf("inspect manifest last_pushed_by column: %w", err)
		}
	default:
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM pragma_table_info('registry_manifests')
			WHERE name = 'last_pushed_by'
		`).Scan(ctx, &count); err != nil {
			return fmt.Errorf("inspect manifest last_pushed_by column: %w", err)
		}
	}
	if count > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		"ALTER TABLE registry_manifests ADD COLUMN last_pushed_by TEXT NOT NULL DEFAULT ''",
	); err != nil {
		return fmt.Errorf("add manifest last_pushed_by column: %w", err)
	}
	return nil
}

func preventManifestLastPushedByRollback(context.Context, *bun.DB) error {
	// SQLite cannot portably drop the column. Keeping the migration applied also
	// prevents a later startup from re-running its forward schema change.
	return fmt.Errorf("registry manifest last_pushed_by migration cannot be rolled back")
}
