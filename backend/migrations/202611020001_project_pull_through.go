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
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'projects'
			  AND column_name = ?
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
