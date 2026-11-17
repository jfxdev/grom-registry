package mcp

import (
	"context"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type principalKey struct{}

// WithPrincipal attaches the access-key principal authenticated by the HTTP
// layer. Tools never fall back to another identity when it is absent.
func WithPrincipal(ctx context.Context, principal foundation.PrincipalRef) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func principalFromContext(ctx context.Context) (foundation.PrincipalRef, error) {
	principal, ok := ctx.Value(principalKey{}).(foundation.PrincipalRef)
	if !ok || principal.Kind == "" || principal.ID == "" {
		return foundation.PrincipalRef{}, errUnauthenticated
	}
	return principal, nil
}
