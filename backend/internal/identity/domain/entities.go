package domain

import (
	"errors"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

var (
	ErrViewerRegistryTokenAlreadyExists = errors.New("an active viewer registry token already exists")
	ErrServiceAccountAccessKeyLimit     = errors.New("a service account can have at most three active access keys")
	ErrUsernameAlreadyExists            = errors.New("username is already in use")
	ErrEmailAlreadyExists               = errors.New("email is already in use")
)

type User struct {
	ID           foundation.ID `json:"id"`
	Email        string        `json:"email"`
	Username     string        `json:"username"`
	PasswordHash string        `json:"-"`
	SystemAdmin  bool          `json:"systemAdmin"`
	SystemViewer bool          `json:"systemViewer"`
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
	ID               foundation.ID           `json:"id"`
	PublicID         string                  `json:"publicId"`
	ServiceAccountID foundation.ID           `json:"serviceAccountId"`
	Principal        foundation.PrincipalRef `json:"-"`
	Name             string                  `json:"name"`
	SecretHash       string                  `json:"-"`
	CreatedAt        time.Time               `json:"createdAt"`
	ExpiresAt        *time.Time              `json:"expiresAt,omitempty"`
	LastUsedAt       *time.Time              `json:"lastUsedAt,omitempty"`
	RevokedAt        *time.Time              `json:"revokedAt,omitempty"`
}

// ViewerRegistryToken is an access key owned by an installation viewer. It can
// only be exchanged for pull access and is deliberately separate from service
// account credentials in the management API.
type ViewerRegistryToken struct {
	ID         foundation.ID `json:"id"`
	PublicID   string        `json:"publicId"`
	Name       string        `json:"name"`
	CreatedAt  time.Time     `json:"createdAt"`
	ExpiresAt  *time.Time    `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time    `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time    `json:"revokedAt,omitempty"`
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
	Purpose    PasswordResetPurpose
}

type PasswordResetPurpose string

const (
	PasswordResetPurposeRegistration  PasswordResetPurpose = "registration"
	PasswordResetPurposePasswordReset PasswordResetPurpose = "password_reset"
)
