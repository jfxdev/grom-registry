package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`CREATE TABLE registry_repositories (
				id TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				name TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				UNIQUE (project_id, name),
				FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_registry_repositories_project
				ON registry_repositories(project_id, name)`,
			`CREATE TABLE repository_policies (
				id TEXT PRIMARY KEY,
				repository_id TEXT NOT NULL,
				type TEXT NOT NULL,
				configuration_json TEXT NOT NULL,
				enabled BOOLEAN NOT NULL DEFAULT TRUE,
				version INTEGER NOT NULL DEFAULT 1,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL,
				FOREIGN KEY (repository_id) REFERENCES registry_repositories(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_repository_policies_repository
				ON repository_policies(repository_id, type)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			"DROP TABLE IF EXISTS repository_policies",
			"DROP TABLE IF EXISTS registry_repositories",
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	})
}
