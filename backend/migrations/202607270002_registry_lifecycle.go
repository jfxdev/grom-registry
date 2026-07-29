package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`CREATE TABLE registry_manifests (
				id TEXT PRIMARY KEY,
				repository_id TEXT NOT NULL,
				digest TEXT NOT NULL,
				media_type TEXT NOT NULL DEFAULT '',
				artifact_type TEXT NOT NULL DEFAULT '',
				subject_digest TEXT NOT NULL DEFAULT '',
				manifest_size BIGINT NOT NULL DEFAULT 0,
				state TEXT NOT NULL,
				first_seen_at TIMESTAMP NOT NULL,
				last_pushed_at TIMESTAMP NULL,
				last_seen_at TIMESTAMP NOT NULL,
				untagged_at TIMESTAMP NULL,
				deleted_at TIMESTAMP NULL,
				UNIQUE (repository_id, digest),
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_registry_manifests_repository_state ON registry_manifests(repository_id, state)`,
			`CREATE INDEX idx_registry_manifests_subject ON registry_manifests(repository_id, subject_digest)`,
			`CREATE TABLE registry_tags (
				repository_id TEXT NOT NULL,
				name TEXT NOT NULL,
				manifest_id TEXT NOT NULL,
				first_seen_at TIMESTAMP NOT NULL,
				last_moved_at TIMESTAMP NOT NULL,
				last_seen_at TIMESTAMP NOT NULL,
				detached_at TIMESTAMP NULL,
				PRIMARY KEY (repository_id, name),
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE,
				FOREIGN KEY (manifest_id) REFERENCES registry_manifests(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_registry_tags_manifest ON registry_tags(manifest_id, detached_at)`,
			`CREATE TABLE lifecycle_previews (
				id TEXT PRIMARY KEY,
				repository_id TEXT NOT NULL,
				policy_snapshot_json TEXT NOT NULL,
				status TEXT NOT NULL,
				inventory_at TIMESTAMP NOT NULL,
				created_by TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				expires_at TIMESTAMP NOT NULL,
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
			)`,
			`CREATE TABLE lifecycle_preview_items (
				id TEXT PRIMARY KEY,
				preview_id TEXT NOT NULL,
				manifest_id TEXT NOT NULL,
				digest TEXT NOT NULL,
				media_type TEXT NOT NULL DEFAULT '',
				artifact_type TEXT NOT NULL DEFAULT '',
				decision TEXT NOT NULL,
				reasons_json TEXT NOT NULL,
				expected_tags_json TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				FOREIGN KEY (preview_id) REFERENCES lifecycle_previews(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_lifecycle_preview_items_preview ON lifecycle_preview_items(preview_id, decision)`,
			`CREATE TABLE lifecycle_runs (
				id TEXT PRIMARY KEY,
				preview_id TEXT NOT NULL,
				repository_id TEXT NOT NULL,
				actor_id TEXT NOT NULL,
				reason TEXT NOT NULL,
				status TEXT NOT NULL,
				started_at TIMESTAMP NOT NULL,
				completed_at TIMESTAMP NULL,
				FOREIGN KEY (preview_id) REFERENCES lifecycle_previews(id),
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_lifecycle_runs_repository ON lifecycle_runs(repository_id, started_at)`,
			`CREATE TABLE lifecycle_run_items (
				id TEXT PRIMARY KEY,
				run_id TEXT NOT NULL,
				digest TEXT NOT NULL,
				status TEXT NOT NULL,
				message TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				FOREIGN KEY (run_id) REFERENCES lifecycle_runs(id) ON DELETE CASCADE
			)`,
			`CREATE TABLE lifecycle_locks (
				repository_id TEXT PRIMARY KEY,
				run_id TEXT NOT NULL,
				acquired_at TIMESTAMP NOT NULL,
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
			)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		for _, statement := range []string{
			"DROP TABLE IF EXISTS lifecycle_locks",
			"DROP TABLE IF EXISTS lifecycle_run_items",
			"DROP TABLE IF EXISTS lifecycle_runs",
			"DROP TABLE IF EXISTS lifecycle_preview_items",
			"DROP TABLE IF EXISTS lifecycle_previews",
			"DROP TABLE IF EXISTS registry_tags",
			"DROP TABLE IF EXISTS registry_manifests",
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	})
}
