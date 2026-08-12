package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(addManifestPlatforms, dropManifestPlatforms)
}

func addManifestPlatforms(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE registry_manifest_platforms (
			manifest_id TEXT NOT NULL,
			digest TEXT NOT NULL,
			os TEXT NOT NULL,
			architecture TEXT NOT NULL,
			variant TEXT NOT NULL DEFAULT '',
			compressed_size BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (manifest_id, digest),
			FOREIGN KEY (manifest_id) REFERENCES registry_manifests(id) ON DELETE CASCADE
		)`)
	return err
}

func dropManifestPlatforms(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS registry_manifest_platforms")
	return err
}
