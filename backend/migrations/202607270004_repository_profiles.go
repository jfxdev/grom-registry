package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		for _, statement := range []string{
			`ALTER TABLE registry_repositories ADD COLUMN profile TEXT NOT NULL DEFAULT 'unknown'`,
			`ALTER TABLE registry_repositories ADD COLUMN profile_source TEXT NOT NULL DEFAULT 'none'`,
			`ALTER TABLE registry_repositories ADD COLUMN profile_confidence TEXT NOT NULL DEFAULT 'none'`,
			`ALTER TABLE registry_repositories ADD COLUMN profile_inferred_at TIMESTAMP NULL`,
			`ALTER TABLE registry_repositories ADD COLUMN profile_needs_review BOOLEAN NOT NULL DEFAULT FALSE`,
			`ALTER TABLE registry_manifests ADD COLUMN observed_kind TEXT NOT NULL DEFAULT 'unknown_oci'`,
			`ALTER TABLE registry_manifests ADD COLUMN artifact_relationship TEXT NOT NULL DEFAULT 'primary'`,
			`ALTER TABLE registry_manifests ADD COLUMN classification_source TEXT NOT NULL DEFAULT 'unknown'`,
			`ALTER TABLE registry_manifests ADD COLUMN classification_confidence TEXT NOT NULL DEFAULT 'none'`,
			`CREATE INDEX idx_registry_manifests_kind ON registry_manifests(repository_id, observed_kind)`,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		// SQLite cannot portably drop columns. Forward migrations are authoritative.
		_, err := db.ExecContext(ctx, "DROP INDEX IF EXISTS idx_registry_manifests_kind")
		return err
	})
}
