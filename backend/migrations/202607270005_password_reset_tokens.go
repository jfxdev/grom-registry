package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`CREATE TABLE password_reset_tokens (
				id TEXT PRIMARY KEY,
				public_id TEXT NOT NULL UNIQUE,
				user_id TEXT NOT NULL,
				secret_hash TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL,
				expires_at TIMESTAMP NOT NULL,
				used_at TIMESTAMP NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX idx_password_reset_tokens_user
				ON password_reset_tokens(user_id)`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		statements := []string{"DROP TABLE IF EXISTS password_reset_tokens"}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	})
}
