package application

import (
	"context"
	"database/sql"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
)

// fakeRepository is an in-memory identity.Repository + identity.PagedRepository
// used to exercise application-service logic without a real database.
type fakeRepository struct {
	users           map[foundation.ID]*identity.User
	sessions        map[string]*identity.Session
	passwordResets  map[string]*identity.PasswordReset
	serviceAccounts map[foundation.ID]*identity.ServiceAccount
	saTokens        map[foundation.ID]*identity.APIToken
	viewerTokens    map[foundation.ID]*identity.APIToken
	viewerTokensByU map[foundation.ID][]foundation.ID
	invalidated     bool
}

var (
	_ identity.Repository      = (*fakeRepository)(nil)
	_ identity.PagedRepository = (*fakeRepository)(nil)
)

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		users:           map[foundation.ID]*identity.User{},
		sessions:        map[string]*identity.Session{},
		passwordResets:  map[string]*identity.PasswordReset{},
		serviceAccounts: map[foundation.ID]*identity.ServiceAccount{},
		saTokens:        map[foundation.ID]*identity.APIToken{},
		viewerTokens:    map[foundation.ID]*identity.APIToken{},
		viewerTokensByU: map[foundation.ID][]foundation.ID{},
	}
}

func (r *fakeRepository) CountUsers(context.Context) (int, error) {
	return len(r.users), nil
}

func (r *fakeRepository) CreateUser(_ context.Context, user *identity.User) error {
	for _, existing := range r.users {
		if existing.Email == user.Email {
			return identity.ErrEmailAlreadyExists
		}
		if existing.Username == user.Username {
			return identity.ErrUsernameAlreadyExists
		}
	}
	copied := *user
	r.users[user.ID] = &copied
	return nil
}

func (r *fakeRepository) CreateUserWithPasswordReset(ctx context.Context, user *identity.User, reset *identity.PasswordReset) error {
	if err := r.CreateUser(ctx, user); err != nil {
		return err
	}
	copied := *reset
	r.passwordResets[reset.PublicID] = &copied
	return nil
}

func (r *fakeRepository) FindUserByEmail(_ context.Context, email string) (*identity.User, error) {
	for _, user := range r.users {
		if user.Email == email && user.DisabledAt == nil {
			return user, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) FindUserByUsername(_ context.Context, username string) (*identity.User, error) {
	for _, user := range r.users {
		if user.Username == username && user.DisabledAt == nil {
			return user, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) FindUserByID(_ context.Context, id foundation.ID) (*identity.User, error) {
	user, ok := r.users[id]
	if !ok || user.DisabledAt != nil {
		return nil, sql.ErrNoRows
	}
	return user, nil
}

func (r *fakeRepository) FindUserByIDIncludingDisabled(_ context.Context, id foundation.ID) (*identity.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return user, nil
}

func (r *fakeRepository) ListUsers(_ context.Context) ([]identity.User, error) {
	result := make([]identity.User, 0, len(r.users))
	for _, user := range r.users {
		result = append(result, *user)
	}
	return result, nil
}

func (r *fakeRepository) PromoteUserToSystemAdmin(_ context.Context, id foundation.ID) error {
	user, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	user.SystemAdmin = true
	user.SystemViewer = false
	return nil
}

func (r *fakeRepository) PromoteUserToSystemViewer(_ context.Context, id foundation.ID) error {
	user, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	user.SystemViewer = true
	return nil
}

func (r *fakeRepository) DisableUser(_ context.Context, id foundation.ID) error {
	user, ok := r.users[id]
	if !ok || user.DisabledAt != nil {
		return sql.ErrNoRows
	}
	now := time.Now().UTC()
	user.DisabledAt = &now
	return nil
}

func (r *fakeRepository) ReactivateUser(_ context.Context, id foundation.ID) error {
	user, ok := r.users[id]
	if !ok || user.DisabledAt == nil {
		return sql.ErrNoRows
	}
	user.DisabledAt = nil
	return nil
}

func (r *fakeRepository) UpdateUser(_ context.Context, id foundation.ID, email, username *string) error {
	user, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	if email != nil {
		user.Email = *email
	}
	if username != nil {
		user.Username = *username
	}
	return nil
}

func (r *fakeRepository) UpdateUserPassword(_ context.Context, id foundation.ID, passwordHash string) error {
	user, ok := r.users[id]
	if !ok {
		return sql.ErrNoRows
	}
	user.PasswordHash = passwordHash
	return nil
}

func (r *fakeRepository) CreatePasswordReset(_ context.Context, reset *identity.PasswordReset) error {
	copied := *reset
	r.passwordResets[reset.PublicID] = &copied
	return nil
}

func (r *fakeRepository) FindPasswordResetByPublicID(_ context.Context, publicID string) (*identity.PasswordReset, error) {
	reset, ok := r.passwordResets[publicID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return reset, nil
}

func (r *fakeRepository) ConsumePasswordReset(_ context.Context, resetID, userID foundation.ID, passwordHash string, activateUser bool, usedAt time.Time) error {
	user, ok := r.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	user.PasswordHash = passwordHash
	if activateUser {
		user.DisabledAt = nil
	}
	for _, reset := range r.passwordResets {
		if reset.ID == resetID {
			reset.UsedAt = &usedAt
		}
	}
	return nil
}

func (r *fakeRepository) CreateSession(_ context.Context, session *identity.Session) error {
	copied := *session
	r.sessions[session.PublicID] = &copied
	return nil
}

func (r *fakeRepository) FindSessionByPublicID(_ context.Context, publicID string) (*identity.Session, error) {
	session, ok := r.sessions[publicID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return session, nil
}

func (r *fakeRepository) DeleteSession(_ context.Context, publicID string) error {
	delete(r.sessions, publicID)
	return nil
}

func (r *fakeRepository) InvalidateEphemeralCredentials(context.Context) error {
	r.invalidated = true
	return nil
}

func (r *fakeRepository) CreateServiceAccount(_ context.Context, account *identity.ServiceAccount) error {
	for _, existing := range r.serviceAccounts {
		if existing.Username == account.Username {
			return identity.ErrUsernameAlreadyExists
		}
	}
	copied := *account
	r.serviceAccounts[account.ID] = &copied
	return nil
}

func (r *fakeRepository) ListServiceAccounts(_ context.Context, includeDisabled bool) ([]identity.ServiceAccount, error) {
	result := make([]identity.ServiceAccount, 0, len(r.serviceAccounts))
	for _, account := range r.serviceAccounts {
		if account.DisabledAt != nil && !includeDisabled {
			continue
		}
		result = append(result, *account)
	}
	return result, nil
}

func (r *fakeRepository) FindServiceAccountByID(_ context.Context, id foundation.ID) (*identity.ServiceAccount, error) {
	account, ok := r.serviceAccounts[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return account, nil
}

func (r *fakeRepository) FindServiceAccountByUsername(_ context.Context, username string) (*identity.ServiceAccount, error) {
	for _, account := range r.serviceAccounts {
		if account.Username == username {
			return account, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) DisableServiceAccount(_ context.Context, id foundation.ID) error {
	account, ok := r.serviceAccounts[id]
	if !ok {
		return sql.ErrNoRows
	}
	now := time.Now().UTC()
	account.DisabledAt = &now
	return nil
}

func (r *fakeRepository) CreateServiceAccountAPIToken(_ context.Context, token *identity.APIToken, _ time.Time, maxActive int) error {
	active := 0
	for _, existing := range r.saTokens {
		if existing.ServiceAccountID == token.ServiceAccountID && existing.RevokedAt == nil {
			active++
		}
	}
	if active >= maxActive {
		return identity.ErrServiceAccountAccessKeyLimit
	}
	copied := *token
	r.saTokens[token.ID] = &copied
	return nil
}

func (r *fakeRepository) ListServiceAccountAPITokens(_ context.Context, serviceAccountID foundation.ID) ([]identity.APIToken, error) {
	result := make([]identity.APIToken, 0)
	for _, token := range r.saTokens {
		if token.ServiceAccountID == serviceAccountID {
			result = append(result, *token)
		}
	}
	return result, nil
}

func (r *fakeRepository) CountActiveServiceAccountAPITokens(_ context.Context, serviceAccountID foundation.ID, now time.Time) (int, error) {
	count := 0
	for _, token := range r.saTokens {
		if token.ServiceAccountID != serviceAccountID {
			continue
		}
		if token.RevokedAt != nil {
			continue
		}
		if token.ExpiresAt != nil && now.After(*token.ExpiresAt) {
			continue
		}
		count++
	}
	return count, nil
}

func (r *fakeRepository) FindAPITokenByPublicID(_ context.Context, publicID string) (*identity.APIToken, error) {
	for _, token := range r.saTokens {
		if token.PublicID == publicID {
			return token, nil
		}
	}
	for _, token := range r.viewerTokens {
		if token.PublicID == publicID {
			return token, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) RevokeServiceAccountAPIToken(_ context.Context, serviceAccountID, tokenID foundation.ID) error {
	token, ok := r.saTokens[tokenID]
	if !ok || token.ServiceAccountID != serviceAccountID {
		return sql.ErrNoRows
	}
	now := time.Now().UTC()
	token.RevokedAt = &now
	return nil
}

func (r *fakeRepository) CreateViewerAPIToken(_ context.Context, token *identity.APIToken) error {
	copied := *token
	r.viewerTokens[token.ID] = &copied
	r.viewerTokensByU[token.Principal.ID] = append(r.viewerTokensByU[token.Principal.ID], token.ID)
	return nil
}

func (r *fakeRepository) ListViewerAPITokens(_ context.Context, userID foundation.ID) ([]identity.ViewerRegistryToken, error) {
	result := make([]identity.ViewerRegistryToken, 0)
	for _, id := range r.viewerTokensByU[userID] {
		token := r.viewerTokens[id]
		result = append(result, identity.ViewerRegistryToken{
			ID: token.ID, PublicID: token.PublicID, Name: token.Name,
			CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt, LastUsedAt: token.LastUsedAt, RevokedAt: token.RevokedAt,
		})
	}
	return result, nil
}

func (r *fakeRepository) RevokeViewerAPIToken(_ context.Context, userID, tokenID foundation.ID) error {
	token, ok := r.viewerTokens[tokenID]
	if !ok || token.Principal.ID != userID {
		return sql.ErrNoRows
	}
	now := time.Now().UTC()
	token.RevokedAt = &now
	return nil
}

func (r *fakeRepository) TouchAPIToken(_ context.Context, id foundation.ID) error {
	now := time.Now().UTC()
	if token, ok := r.saTokens[id]; ok {
		token.LastUsedAt = &now
	}
	if token, ok := r.viewerTokens[id]; ok {
		token.LastUsedAt = &now
	}
	return nil
}

func (r *fakeRepository) ListUsersPage(_ context.Context, _ string, request foundation.PageRequest) (foundation.PageResult[identity.User], error) {
	users, _ := r.ListUsers(context.Background())
	if request.Limit > 0 && len(users) > request.Limit {
		users = users[:request.Limit]
	}
	return foundation.PageResult[identity.User]{Items: users}, nil
}

func (r *fakeRepository) ListServiceAccountsPage(_ context.Context, _, _ string, request foundation.PageRequest) (foundation.PageResult[identity.ServiceAccount], error) {
	accounts, _ := r.ListServiceAccounts(context.Background(), true)
	if request.Limit > 0 && len(accounts) > request.Limit {
		accounts = accounts[:request.Limit]
	}
	return foundation.PageResult[identity.ServiceAccount]{Items: accounts}, nil
}

func (r *fakeRepository) ListServiceAccountAPITokensPage(_ context.Context, serviceAccountID foundation.ID, request foundation.PageRequest) (foundation.PageResult[identity.APIToken], error) {
	tokens, _ := r.ListServiceAccountAPITokens(context.Background(), serviceAccountID)
	if request.Limit > 0 && len(tokens) > request.Limit {
		tokens = tokens[:request.Limit]
	}
	return foundation.PageResult[identity.APIToken]{Items: tokens}, nil
}
