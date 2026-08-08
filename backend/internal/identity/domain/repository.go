package domain

import (
	"context"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Repository interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, user *User) error
	CreateUserWithPasswordReset(ctx context.Context, user *User, reset *PasswordReset) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByUsername(ctx context.Context, username string) (*User, error)
	FindUserByID(ctx context.Context, id foundation.ID) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	PromoteUserToSystemAdmin(ctx context.Context, id foundation.ID) error
	PromoteUserToSystemViewer(ctx context.Context, id foundation.ID) error
	DisableUser(ctx context.Context, id foundation.ID) error
	UpdateUserPassword(ctx context.Context, id foundation.ID, passwordHash string) error

	CreatePasswordReset(ctx context.Context, reset *PasswordReset) error
	FindPasswordResetByPublicID(ctx context.Context, publicID string) (*PasswordReset, error)
	ConsumePasswordReset(ctx context.Context, resetID, userID foundation.ID, passwordHash string, activateUser bool, usedAt time.Time) error

	CreateSession(ctx context.Context, session *Session) error
	FindSessionByPublicID(ctx context.Context, publicID string) (*Session, error)
	DeleteSession(ctx context.Context, publicID string) error
	InvalidateEphemeralCredentials(ctx context.Context) error

	CreateServiceAccount(ctx context.Context, account *ServiceAccount) error
	ListServiceAccounts(ctx context.Context, includeDisabled bool) ([]ServiceAccount, error)
	FindServiceAccountByID(ctx context.Context, id foundation.ID) (*ServiceAccount, error)
	FindServiceAccountByUsername(ctx context.Context, username string) (*ServiceAccount, error)
	DisableServiceAccount(ctx context.Context, id foundation.ID) error

	CreateServiceAccountAPIToken(ctx context.Context, token *APIToken) error
	ListServiceAccountAPITokens(ctx context.Context, serviceAccountID foundation.ID) ([]APIToken, error)
	FindAPITokenByPublicID(ctx context.Context, publicID string) (*APIToken, error)
	RevokeServiceAccountAPIToken(ctx context.Context, serviceAccountID, tokenID foundation.ID) error
	CreateViewerAPIToken(ctx context.Context, token *APIToken) error
	ListViewerAPITokens(ctx context.Context, userID foundation.ID) ([]ViewerRegistryToken, error)
	RevokeViewerAPIToken(ctx context.Context, userID, tokenID foundation.ID) error
	TouchAPIToken(ctx context.Context, id foundation.ID) error
}
