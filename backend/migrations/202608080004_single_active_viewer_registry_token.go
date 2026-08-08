package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Collection.MustRegister(addSingleActiveViewerRegistryTokenIndex, func(context.Context, *bun.DB) error {
		return nil
	})
}

func addSingleActiveViewerRegistryTokenIndex(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, "CREATE UNIQUE INDEX idx_api_tokens_single_active_viewer ON api_tokens (principal_kind, principal_id) WHERE principal_kind = 'user' AND revoked_at IS NULL")
	return err
}
