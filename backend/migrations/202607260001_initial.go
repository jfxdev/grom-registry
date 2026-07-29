package migrations

import (
	"context"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

var Collection = migrate.NewMigrations()

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`CREATE TABLE users (
				id TEXT PRIMARY KEY,
				email TEXT NOT NULL UNIQUE,
				username TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL,
				is_system_admin BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMP NOT NULL,
				disabled_at TIMESTAMP NULL
			)`,
			`CREATE TABLE service_accounts (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				username TEXT NOT NULL UNIQUE,
				description TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMP NOT NULL,
				disabled_at TIMESTAMP NULL
			)`,
			`CREATE TABLE projects (
				id TEXT PRIMARY KEY,
				slug TEXT NOT NULL UNIQUE,
				name TEXT NOT NULL,
				created_by TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL
			)`,
			`CREATE TABLE project_memberships (
				project_id TEXT NOT NULL,
				principal_kind TEXT NOT NULL,
				principal_id TEXT NOT NULL,
				role TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				PRIMARY KEY (project_id, principal_kind, principal_id),
				FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_project_memberships_principal
				ON project_memberships(principal_kind, principal_id)`,
			`CREATE TABLE api_tokens (
				id TEXT PRIMARY KEY,
				public_id TEXT NOT NULL UNIQUE,
				principal_kind TEXT NOT NULL,
				principal_id TEXT NOT NULL,
				name TEXT NOT NULL,
				secret_hash TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				expires_at TIMESTAMP NULL,
				last_used_at TIMESTAMP NULL,
				revoked_at TIMESTAMP NULL
			)`,
			`CREATE INDEX idx_api_tokens_principal
				ON api_tokens(principal_kind, principal_id)`,
			`CREATE TABLE sessions (
				id TEXT PRIMARY KEY,
				public_id TEXT NOT NULL UNIQUE,
				user_id TEXT NOT NULL,
				secret_hash TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				expires_at TIMESTAMP NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
			`CREATE TABLE audit_events (
				id TEXT PRIMARY KEY,
				actor_kind TEXT NOT NULL,
				actor_id TEXT NOT NULL,
				action TEXT NOT NULL,
				resource_kind TEXT NOT NULL,
				resource_id TEXT NOT NULL,
				metadata_json TEXT NOT NULL DEFAULT '{}',
				created_at TIMESTAMP NOT NULL
			)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			"DROP TABLE IF EXISTS audit_events",
			"DROP TABLE IF EXISTS sessions",
			"DROP TABLE IF EXISTS api_tokens",
			"DROP TABLE IF EXISTS project_memberships",
			"DROP TABLE IF EXISTS projects",
			"DROP TABLE IF EXISTS service_accounts",
			"DROP TABLE IF EXISTS users",
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	})
}
