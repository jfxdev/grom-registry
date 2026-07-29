package domain

import (
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type User struct {
	ID           foundation.ID `json:"id"`
	Email        string        `json:"email"`
	Username     string        `json:"username"`
	PasswordHash string        `json:"-"`
	SystemAdmin  bool          `json:"systemAdmin"`
	CreatedAt    time.Time     `json:"createdAt"`
	DisabledAt   *time.Time    `json:"disabledAt,omitempty"`
}

type ServiceAccount struct {
	ID          foundation.ID `json:"id"`
	Name        string        `json:"name"`
	Username    string        `json:"username"`
	Description string        `json:"description"`
	CreatedAt   time.Time     `json:"createdAt"`
	DisabledAt  *time.Time    `json:"disabledAt,omitempty"`
}

type APIToken struct {
	ID               foundation.ID `json:"id"`
	PublicID         string        `json:"publicId"`
	ServiceAccountID foundation.ID `json:"serviceAccountId"`
	Name             string        `json:"name"`
	SecretHash       string        `json:"-"`
	CreatedAt        time.Time     `json:"createdAt"`
	ExpiresAt        *time.Time    `json:"expiresAt,omitempty"`
	LastUsedAt       *time.Time    `json:"lastUsedAt,omitempty"`
	RevokedAt        *time.Time    `json:"revokedAt,omitempty"`
}

type Session struct {
	ID         foundation.ID `json:"id"`
	PublicID   string
	UserID     foundation.ID
	SecretHash string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

type PasswordReset struct {
	ID         foundation.ID
	PublicID   string
	UserID     foundation.ID
	SecretHash string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	UsedAt     *time.Time
}
