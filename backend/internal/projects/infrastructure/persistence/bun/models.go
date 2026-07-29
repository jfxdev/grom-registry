package bunstore

import (
	"time"

	"github.com/uptrace/bun"
)

type projectModel struct {
	bun.BaseModel `bun:"table:projects,alias:p"`
	ID            string    `bun:"id,pk"`
	Slug          string    `bun:"slug,notnull,unique"`
	Name          string    `bun:"name,notnull"`
	CreatedBy     string    `bun:"created_by,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull"`
}

type membershipModel struct {
	bun.BaseModel `bun:"table:project_memberships,alias:m"`
	ProjectID     string    `bun:"project_id,pk"`
	PrincipalKind string    `bun:"principal_kind,pk"`
	PrincipalID   string    `bun:"principal_id,pk"`
	Role          string    `bun:"role,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull"`
}
