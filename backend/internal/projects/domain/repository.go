package domain

import (
	"context"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Repository interface {
	CreateProject(ctx context.Context, project *Project, membership *Membership) error
	DeleteProject(ctx context.Context, projectID foundation.ID) error
	ListProjectsForPrincipal(ctx context.Context, principal foundation.PrincipalRef, systemAdmin bool) ([]Project, error)
	FindProjectBySlug(ctx context.Context, slug string) (*Project, error)
	ListMemberships(ctx context.Context, projectID foundation.ID) ([]Membership, error)
	UpsertMembership(ctx context.Context, membership *Membership) error
	DeleteMembership(ctx context.Context, projectID foundation.ID, principal foundation.PrincipalRef) error
	FindMembership(ctx context.Context, projectID foundation.ID, principal foundation.PrincipalRef) (*Membership, error)
}
