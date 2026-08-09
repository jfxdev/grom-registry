package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

// Repair installations that recorded the original policy migration before it
// gained the policy_version column. Migrations are append-only, so this must
// remain a separate forward repair rather than changing an applied migration.
func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		return ensureRepositoryPolicyVersionColumn(ctx, db)
	}, func(context.Context, *bun.DB) error {
		// SQLite cannot portably drop a column on every supported version.
		return nil
	})
}

func ensureRepositoryPolicyVersionColumn(ctx context.Context, db *bun.DB) error {
	var count int
	switch db.Dialect().Name() {
	case dialect.PG:
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'registry_repositories'
			  AND column_name = 'policy_version'
		`).Scan(ctx, &count); err != nil {
			return fmt.Errorf("inspect repository policy version column: %w", err)
		}
	default:
		if err := db.NewRaw(`
			SELECT COUNT(*)
			FROM pragma_table_info('registry_repositories')
			WHERE name = 'policy_version'
		`).Scan(ctx, &count); err != nil {
			return fmt.Errorf("inspect repository policy version column: %w", err)
		}
	}
	if count > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		"ALTER TABLE registry_repositories ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 0",
	); err != nil {
		return fmt.Errorf("add repository policy version column: %w", err)
	}
	return nil
}
