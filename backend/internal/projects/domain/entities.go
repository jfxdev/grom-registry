package domain

import (
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Project struct {
	ID             foundation.ID                    `json:"id"`
	Slug           string                           `json:"slug"`
	Name           string                           `json:"name"`
	CreatedBy      foundation.ID                    `json:"createdBy"`
	CreatedAt      time.Time                        `json:"createdAt"`
	AccountedUsage foundation.AccountedStorageUsage `json:"accountedUsage"`
}

type Membership struct {
	ProjectID     foundation.ID `json:"projectId"`
	PrincipalKind string        `json:"principalKind"`
	PrincipalID   foundation.ID `json:"principalId"`
	Role          string        `json:"role"`
	CreatedAt     time.Time     `json:"createdAt"`
}
