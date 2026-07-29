package migrations

import (
	"context"
	"errors"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`ALTER TABLE registry_repositories ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 0`,
			`UPDATE registry_repositories
			 SET policy_version = 1
			 WHERE EXISTS (
				SELECT 1 FROM repository_policies
				WHERE repository_policies.repository_id = registry_repositories.id
			 )`,
			`ALTER TABLE lifecycle_previews ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE lifecycle_previews ADD COLUMN evaluator_version INTEGER NOT NULL DEFAULT 1`,
			`CREATE TABLE artifact_deletions (
				id TEXT PRIMARY KEY,
				repository_id TEXT NOT NULL,
				digest TEXT NOT NULL,
				affected_tags_json TEXT NOT NULL,
				actor_id TEXT NOT NULL,
				reason TEXT NOT NULL,
				status TEXT NOT NULL,
				message TEXT NOT NULL DEFAULT '',
				started_at TIMESTAMP NOT NULL,
				completed_at TIMESTAMP NULL,
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_artifact_deletions_repository ON artifact_deletions(repository_id, started_at)`,
			`CREATE UNIQUE INDEX idx_lifecycle_runs_preview ON lifecycle_runs(preview_id)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}, func(context.Context, *bun.DB) error {
		// SQLite cannot portably drop columns on every supported version.
		return errors.New("migration 202607270003 rollback is unsupported")
	})
}
