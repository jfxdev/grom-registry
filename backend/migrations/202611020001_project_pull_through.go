package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

func init() {
	Collection.MustRegister(ensureProjectPullThroughColumns, preventProjectPullThroughRollback)
}

func ensureProjectPullThroughColumns(ctx context.Context, db *bun.DB) error {
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "mode", definition: "TEXT NOT NULL DEFAULT 'standard'"},
		{name: "upstream", definition: "TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := projectColumnExists(ctx, db, column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.ExecContext(ctx, "ALTER TABLE projects ADD COLUMN "+column.name+" "+column.definition); err != nil {
			return fmt.Errorf("add project %s column: %w", column.name, err)
		}
	}
	return nil
}

func projectColumnExists(ctx context.Context, db *bun.DB, column string) (bool, error) {
	var count int
	switch db.Dialect().Name() {
	case dialect.PG:
		// to_regclass resolves projects through search_path exactly like the
		// following ALTER TABLE, so an earlier schema cannot hide the table.
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM pg_attribute
			WHERE attrelid = to_regclass('projects')
			  AND attname = ?
			  AND NOT attisdropped
		`, column).Scan(ctx, &count); err != nil {
			return false, fmt.Errorf("inspect project %s column: %w", column, err)
		}
	default:
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM pragma_table_info('projects')
			WHERE name = ?
		`, column).Scan(ctx, &count); err != nil {
			return false, fmt.Errorf("inspect project %s column: %w", column, err)
		}
	}
	return count > 0, nil
}

func preventProjectPullThroughRollback(context.Context, *bun.DB) error {
	return fmt.Errorf("project pull-through migration cannot be rolled back")
}
