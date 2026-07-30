package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/jfxdev/grom/backend/internal/identity/infrastructure/password"
)

var (
	ErrUnauthenticated        = errors.New("authentication failed")
	ErrInvalidCurrentPassword = errors.New("current password is incorrect")
	ErrInvalidPasswordReset   = errors.New("password reset link is invalid or expired")
)

const dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type Service struct {
	repository identity.Repository
	sessionTTL time.Duration
}

type CreatedToken struct {
	Token  identity.APIToken `json:"token"`
	Secret string            `json:"secret"`
}

type CreatedPasswordReset struct {
	Token     string
	ExpiresAt time.Time
}

func New(repository identity.Repository, sessionTTL time.Duration) *Service {
	return &Service{repository: repository, sessionTTL: sessionTTL}
}

func (s *Service) BootstrapAdmin(ctx context.Context, email, username, plaintext string) error {
	count, err := s.repository.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := password.Hash(plaintext)
	if err != nil {
		return err
	}
	return s.repository.CreateUser(ctx, &identity.User{
		ID: foundation.NewID(), Email: strings.ToLower(strings.TrimSpace(email)),
		Username: strings.TrimSpace(username), PasswordHash: hash,
		SystemAdmin: true, CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) Login(ctx context.Context, email, plaintext string) (string, *identity.User, error) {
	user, lookupErr := s.repository.FindUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	passwordHash := dummyPasswordHash
	if lookupErr == nil && user != nil {
		passwordHash = user.PasswordHash
	}
	passwordMatches := password.Verify(passwordHash, plaintext)
	if lookupErr != nil || user == nil || !passwordMatches {
		return "", nil, ErrUnauthenticated
	}
	publicID, err := randomString(12)
	if err != nil {
		return "", nil, err
	}
	secret, err := randomString(32)
	if err != nil {
		return "", nil, err
	}
	hash, err := password.Hash(secret)
	if err != nil {
		return "", nil, err
	}
	session := &identity.Session{
		ID: foundation.NewID(), PublicID: publicID, UserID: user.ID,
		SecretHash: hash, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(s.sessionTTL),
	}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return "", nil, err
	}
	return "grms_" + publicID + "_" + secret, user, nil
}

func (s *Service) AuthenticateSession(ctx context.Context, raw string) (*identity.User, error) {
	publicID, secret, ok := parseCredential(raw, "grms")
	if !ok {
		return nil, ErrUnauthenticated
	}
	session, err := s.repository.FindSessionByPublicID(ctx, publicID)
	if err != nil || time.Now().UTC().After(session.ExpiresAt) || !password.Verify(session.SecretHash, secret) {
		return nil, ErrUnauthenticated
	}
	user, err := s.repository.FindUserByID(ctx, session.UserID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	publicID, _, ok := parseCredential(raw, "grms")
	if !ok {
		return nil
	}
	return s.repository.DeleteSession(ctx, publicID)
}

func (s *Service) InvalidateEphemeralCredentials(ctx context.Context) error {
	return s.repository.InvalidateEphemeralCredentials(ctx)
}

func (s *Service) ListUsers(ctx context.Context) ([]identity.User, error) {
	return s.repository.ListUsers(ctx)
}

func (s *Service) CreateUser(ctx context.Context, email, username, plaintext string, systemAdmin bool) (*identity.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	username = strings.TrimSpace(username)
	if email == "" || username == "" || len(plaintext) < constants.MinimumPasswordLength {
		return nil, fmt.Errorf("email, username, and a password with at least %d characters are required", constants.MinimumPasswordLength)
	}
	hash, err := password.Hash(plaintext)
	if err != nil {
		return nil, err
	}
	user := &identity.User{
		ID: foundation.NewID(), Email: email, Username: username, PasswordHash: hash,
		SystemAdmin: systemAdmin, CreatedAt: time.Now().UTC(),
	}
	if err := s.repository.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) FindUser(ctx context.Context, id foundation.ID) (*identity.User, error) {
	return s.repository.FindUserByID(ctx, id)
}

func (s *Service) ChangePassword(ctx context.Context, userID foundation.ID, currentPassword, newPassword string) error {
	if len(newPassword) < constants.MinimumPasswordLength {
		return fmt.Errorf("new password must contain at least %d characters", constants.MinimumPasswordLength)
	}
	user, err := s.repository.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !password.Verify(user.PasswordHash, currentPassword) {
		return ErrInvalidCurrentPassword
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	return s.repository.UpdateUserPassword(ctx, userID, hash)
}

func (s *Service) CreatePasswordReset(ctx context.Context, userID foundation.ID) (*CreatedPasswordReset, error) {
	if _, err := s.repository.FindUserByID(ctx, userID); err != nil {
		return nil, err
	}
	publicID, err := randomString(12)
	if err != nil {
		return nil, err
	}
	secret, err := randomString(32)
	if err != nil {
		return nil, err
	}
	hash, err := password.Hash(secret)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(constants.DefaultPasswordResetMinutes * time.Minute)
	reset := &identity.PasswordReset{
		ID: foundation.NewID(), PublicID: publicID, UserID: userID, SecretHash: hash,
		CreatedAt: now, ExpiresAt: expiresAt,
	}
	if err := s.repository.CreatePasswordReset(ctx, reset); err != nil {
		return nil, err
	}
	return &CreatedPasswordReset{Token: "grmpr_" + publicID + "_" + secret, ExpiresAt: expiresAt}, nil
}

func (s *Service) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) (foundation.ID, error) {
	if len(newPassword) < constants.MinimumPasswordLength {
		return "", fmt.Errorf("new password must contain at least %d characters", constants.MinimumPasswordLength)
	}
	publicID, secret, ok := parseCredential(rawToken, "grmpr")
	if !ok {
		return "", ErrInvalidPasswordReset
	}
	reset, err := s.repository.FindPasswordResetByPublicID(ctx, publicID)
	now := time.Now().UTC()
	if err != nil || reset.UsedAt != nil || !now.Before(reset.ExpiresAt) || !password.Verify(reset.SecretHash, secret) {
		return "", ErrInvalidPasswordReset
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return "", err
	}
	if err := s.repository.ConsumePasswordReset(ctx, reset.ID, reset.UserID, hash, now); err != nil {
		return "", ErrInvalidPasswordReset
	}
	return reset.UserID, nil
}

func (s *Service) CreateServiceAccount(ctx context.Context, name, username, description string) (*identity.ServiceAccount, error) {
	account := &identity.ServiceAccount{
		ID: foundation.NewID(), Name: strings.TrimSpace(name), Username: strings.TrimSpace(username),
		Description: strings.TrimSpace(description), CreatedAt: time.Now().UTC(),
	}
	if account.Name == "" || account.Username == "" {
		return nil, fmt.Errorf("name and username are required")
	}
	if err := s.repository.CreateServiceAccount(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

func (s *Service) ListServiceAccounts(ctx context.Context, includeDisabled bool) ([]identity.ServiceAccount, error) {
	return s.repository.ListServiceAccounts(ctx, includeDisabled)
}

func (s *Service) FindServiceAccount(ctx context.Context, id foundation.ID) (*identity.ServiceAccount, error) {
	return s.repository.FindServiceAccountByID(ctx, id)
}

func (s *Service) DisableServiceAccount(ctx context.Context, id foundation.ID) error {
	return s.repository.DisableServiceAccount(ctx, id)
}

func (s *Service) CreateServiceAccountAPIToken(ctx context.Context, serviceAccountID foundation.ID, name string, expiresAt *time.Time) (*CreatedToken, error) {
	if _, err := s.repository.FindServiceAccountByID(ctx, serviceAccountID); err != nil {
		return nil, err
	}
	publicID, err := randomString(10)
	if err != nil {
		return nil, err
	}
	secret, err := randomString(32)
	if err != nil {
		return nil, err
	}
	hash, err := password.Hash(secret)
	if err != nil {
		return nil, err
	}
	token := identity.APIToken{
		ID: foundation.NewID(), PublicID: publicID, ServiceAccountID: serviceAccountID,
		Name: strings.TrimSpace(name), SecretHash: hash,
		CreatedAt: time.Now().UTC(), ExpiresAt: expiresAt,
	}
	if token.Name == "" {
		return nil, fmt.Errorf("token name is required")
	}
	if err := s.repository.CreateServiceAccountAPIToken(ctx, &token); err != nil {
		return nil, err
	}
	return &CreatedToken{Token: token, Secret: "grm_" + publicID + "_" + secret}, nil
}

func (s *Service) ListServiceAccountAPITokens(ctx context.Context, serviceAccountID foundation.ID) ([]identity.APIToken, error) {
	if _, err := s.repository.FindServiceAccountByID(ctx, serviceAccountID); err != nil {
		return nil, err
	}
	return s.repository.ListServiceAccountAPITokens(ctx, serviceAccountID)
}

func (s *Service) RevokeServiceAccountAPIToken(ctx context.Context, serviceAccountID, tokenID foundation.ID) error {
	return s.repository.RevokeServiceAccountAPIToken(ctx, serviceAccountID, tokenID)
}

func (s *Service) AuthenticateRegistry(ctx context.Context, username, rawToken string) (foundation.PrincipalRef, error) {
	publicID, secret, ok := parseCredential(rawToken, "grm")
	if !ok {
		return foundation.PrincipalRef{}, ErrUnauthenticated
	}
	token, err := s.repository.FindAPITokenByPublicID(ctx, publicID)
	if err != nil || token.RevokedAt != nil || (token.ExpiresAt != nil && time.Now().UTC().After(*token.ExpiresAt)) {
		return foundation.PrincipalRef{}, ErrUnauthenticated
	}
	if !password.Verify(token.SecretHash, secret) {
		return foundation.PrincipalRef{}, ErrUnauthenticated
	}
	account, err := s.repository.FindServiceAccountByID(ctx, token.ServiceAccountID)
	if err != nil || account.Username != username {
		return foundation.PrincipalRef{}, ErrUnauthenticated
	}
	_ = s.repository.TouchAPIToken(ctx, token.ID)
	return foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: token.ServiceAccountID}, nil
}

func randomString(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func parseCredential(raw, prefix string) (string, string, bool) {
	parts := strings.SplitN(raw, "_", 3)
	if len(parts) != 3 || parts[0] != prefix || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}
