package bunstore

import (
	"time"

	"github.com/uptrace/bun"
)

type userModel struct {
	bun.BaseModel `bun:"table:users,alias:u"`
	ID            string     `bun:"id,pk"`
	Email         string     `bun:"email,notnull,unique"`
	Username      string     `bun:"username,notnull,unique"`
	Password      string     `bun:"password_hash,notnull"`
	SystemAdmin   bool       `bun:"is_system_admin,notnull"`
	SystemViewer  bool       `bun:"is_system_viewer,notnull"`
	CreatedAt     time.Time  `bun:"created_at,notnull"`
	DisabledAt    *time.Time `bun:"disabled_at"`
}

type serviceAccountModel struct {
	bun.BaseModel `bun:"table:service_accounts,alias:sa"`
	ID            string     `bun:"id,pk"`
	Name          string     `bun:"name,notnull"`
	Username      string     `bun:"username,notnull,unique"`
	Description   string     `bun:"description,notnull"`
	CreatedAt     time.Time  `bun:"created_at,notnull"`
	DisabledAt    *time.Time `bun:"disabled_at"`
}

type apiTokenModel struct {
	bun.BaseModel `bun:"table:api_tokens,alias:t"`
	ID            string     `bun:"id,pk"`
	PublicID      string     `bun:"public_id,notnull,unique"`
	PrincipalKind string     `bun:"principal_kind,notnull"`
	PrincipalID   string     `bun:"principal_id,notnull"`
	Name          string     `bun:"name,notnull"`
	SecretHash    string     `bun:"secret_hash,notnull"`
	CreatedAt     time.Time  `bun:"created_at,notnull"`
	ExpiresAt     *time.Time `bun:"expires_at"`
	LastUsedAt    *time.Time `bun:"last_used_at"`
	RevokedAt     *time.Time `bun:"revoked_at"`
}

type sessionModel struct {
	bun.BaseModel `bun:"table:sessions,alias:s"`
	ID            string    `bun:"id,pk"`
	PublicID      string    `bun:"public_id,notnull,unique"`
	UserID        string    `bun:"user_id,notnull"`
	SecretHash    string    `bun:"secret_hash,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull"`
	ExpiresAt     time.Time `bun:"expires_at,notnull"`
}

type passwordResetModel struct {
	bun.BaseModel `bun:"table:password_reset_tokens,alias:prt"`
	ID            string     `bun:"id,pk"`
	PublicID      string     `bun:"public_id,notnull,unique"`
	UserID        string     `bun:"user_id,notnull"`
	SecretHash    string     `bun:"secret_hash,notnull"`
	CreatedAt     time.Time  `bun:"created_at,notnull"`
	ExpiresAt     time.Time  `bun:"expires_at,notnull"`
	UsedAt        *time.Time `bun:"used_at"`
	Purpose       string     `bun:"purpose,notnull"`
}
