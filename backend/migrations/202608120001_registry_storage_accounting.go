package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(addRegistryStorageAccounting, dropRegistryStorageAccounting)
}

func addRegistryStorageAccounting(ctx context.Context, db *bun.DB) error {
	for _, statement := range []string{
		`CREATE TABLE registry_blob_descriptors (
			digest TEXT PRIMARY KEY,
			size_bytes BIGINT NOT NULL,
			media_type TEXT NOT NULL DEFAULT '',
			first_seen_at TIMESTAMP NOT NULL,
			last_seen_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE registry_manifest_blob_references (
			repository_id TEXT NOT NULL,
			manifest_digest TEXT NOT NULL,
			blob_digest TEXT NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY (repository_id, manifest_digest, blob_digest),
			FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE,
			FOREIGN KEY (blob_digest) REFERENCES registry_blob_descriptors(digest) ON DELETE RESTRICT
		)`,
		`CREATE INDEX idx_registry_manifest_blob_references_blob ON registry_manifest_blob_references(blob_digest)`,
		`CREATE TABLE repository_storage_snapshots (
			repository_id TEXT PRIMARY KEY,
			accounted_bytes BIGINT NOT NULL,
			inventory_version BIGINT NOT NULL,
			reconciled_at TIMESTAMP NOT NULL,
			status TEXT NOT NULL,
			FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE project_storage_snapshots (
			project_id TEXT PRIMARY KEY,
			accounted_bytes BIGINT NOT NULL,
			accounting_version BIGINT NOT NULL,
			reconciled_at TIMESTAMP NOT NULL,
			status TEXT NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func dropRegistryStorageAccounting(ctx context.Context, db *bun.DB) error {
	for _, statement := range []string{
		"DROP TABLE IF EXISTS project_storage_snapshots",
		"DROP TABLE IF EXISTS repository_storage_snapshots",
		"DROP TABLE IF EXISTS registry_manifest_blob_references",
		"DROP TABLE IF EXISTS registry_blob_descriptors",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
