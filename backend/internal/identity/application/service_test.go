package application

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
)

type missingUserRepository struct {
	identity.Repository
}

type createUserRepository struct {
	identity.Repository
	created *identity.User
}

func (r *createUserRepository) CreateUserWithPasswordReset(_ context.Context, user *identity.User, _ *identity.PasswordReset) error {
	r.created = user
	return nil
}

func TestCreateUserAlwaysCreatesARegularUser(t *testing.T) {
	repository := &createUserRepository{}
	service := New(repository, time.Hour)

	created, err := service.CreateUser(context.Background(), "member@example.com", "member")

	if err != nil {
		t.Fatal(err)
	}
	if created.User.SystemAdmin || repository.created.SystemAdmin {
		t.Fatal("expected a regular user")
	}
	if created.User.DisabledAt == nil {
		t.Fatal("expected user registration to start disabled")
	}
	if created.RegistrationLink.Token == "" {
		t.Fatal("expected a registration link token")
	}
}

func TestCreateUserRejectsMissingRequiredFields(t *testing.T) {
	created, err := New(&createUserRepository{}, time.Hour).CreateUser(context.Background(), "", "member")

	if !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("expected ErrInvalidUserInput, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected no user, got %#v", created)
	}
}

type promoteUserRepository struct {
	identity.Repository
	user     *identity.User
	promoted foundation.ID
}

func (r *promoteUserRepository) FindUserByID(_ context.Context, id foundation.ID) (*identity.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, sql.ErrNoRows
	}
	return r.user, nil
}

func (r *promoteUserRepository) PromoteUserToSystemAdmin(_ context.Context, id foundation.ID) error {
	r.promoted = id
	return nil
}

func TestPromoteUserToSystemAdmin(t *testing.T) {
	user := &identity.User{ID: foundation.NewID()}
	repository := &promoteUserRepository{user: user}

	promoted, err := New(repository, time.Hour).PromoteUserToSystemAdmin(context.Background(), user.ID)

	if err != nil {
		t.Fatal(err)
	}
	if !promoted.SystemAdmin || repository.promoted != user.ID {
		t.Fatalf("expected persisted user promotion, got %#v", promoted)
	}
}

type reactivateUserRepository struct {
	identity.Repository
	user             *identity.User
	reactivated      foundation.ID
	reactivateCalled bool
}

func (r *reactivateUserRepository) FindUserByIDIncludingDisabled(_ context.Context, id foundation.ID) (*identity.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, sql.ErrNoRows
	}
	return r.user, nil
}

func (r *reactivateUserRepository) ReactivateUser(_ context.Context, id foundation.ID) error {
	r.reactivateCalled = true
	r.reactivated = id
	return nil
}

func TestReactivateUserClearsDisabledAt(t *testing.T) {
	now := time.Now().UTC()
	user := &identity.User{ID: foundation.NewID(), DisabledAt: &now}
	repository := &reactivateUserRepository{user: user}

	reactivated, err := New(repository, time.Hour).ReactivateUser(context.Background(), user.ID)

	if err != nil {
		t.Fatal(err)
	}
	if reactivated.DisabledAt != nil {
		t.Fatalf("expected DisabledAt to be cleared, got %#v", reactivated.DisabledAt)
	}
	if !repository.reactivateCalled || repository.reactivated != user.ID {
		t.Fatalf("expected repository ReactivateUser to be called with %s", user.ID)
	}
}

func TestReactivateUserIsIdempotentForActiveUser(t *testing.T) {
	user := &identity.User{ID: foundation.NewID()}
	repository := &reactivateUserRepository{user: user}

	reactivated, err := New(repository, time.Hour).ReactivateUser(context.Background(), user.ID)

	if err != nil {
		t.Fatal(err)
	}
	if reactivated.DisabledAt != nil {
		t.Fatalf("expected already-active user to remain active, got %#v", reactivated.DisabledAt)
	}
	if repository.reactivateCalled {
		t.Fatal("expected no repository write for an already-active user")
	}
}

func TestReactivateUserPropagatesNotFound(t *testing.T) {
	repository := &reactivateUserRepository{}

	_, err := New(repository, time.Hour).ReactivateUser(context.Background(), foundation.NewID())

	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

type updateUserRepository struct {
	identity.Repository
	user            *identity.User
	updateErr       error
	updatedEmail    *string
	updatedUsername *string
}

func (r *updateUserRepository) FindUserByIDIncludingDisabled(_ context.Context, id foundation.ID) (*identity.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, sql.ErrNoRows
	}
	return r.user, nil
}

func (r *updateUserRepository) UpdateUser(_ context.Context, _ foundation.ID, email, username *string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updatedEmail = email
	r.updatedUsername = username
	return nil
}

func TestUpdateUserPersistsChangedFields(t *testing.T) {
	user := &identity.User{ID: foundation.NewID(), Email: "old@example.com", Username: "old"}
	repository := &updateUserRepository{user: user}
	newEmail := "New@Example.com"
	newUsername := "newname"

	updated, err := New(repository, time.Hour).UpdateUser(context.Background(), user.ID, &newEmail, &newUsername)

	if err != nil {
		t.Fatal(err)
	}
	if updated.Email != "new@example.com" || updated.Username != "newname" {
		t.Fatalf("expected normalized updated fields, got %#v", updated)
	}
	if repository.updatedEmail == nil || *repository.updatedEmail != "new@example.com" {
		t.Fatalf("expected repository to receive normalized email, got %#v", repository.updatedEmail)
	}
}

func TestUpdateUserRejectsBlankFields(t *testing.T) {
	user := &identity.User{ID: foundation.NewID(), Email: "old@example.com", Username: "old"}
	repository := &updateUserRepository{user: user}
	blank := "   "

	_, err := New(repository, time.Hour).UpdateUser(context.Background(), user.ID, &blank, nil)

	if !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("expected ErrInvalidUserInput, got %v", err)
	}
}

func TestUpdateUserRejectsWhenNoFieldsProvided(t *testing.T) {
	user := &identity.User{ID: foundation.NewID(), Email: "old@example.com", Username: "old"}
	repository := &updateUserRepository{user: user}

	_, err := New(repository, time.Hour).UpdateUser(context.Background(), user.ID, nil, nil)

	if !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("expected ErrInvalidUserInput, got %v", err)
	}
}

func TestUpdateUserPropagatesConflictErrors(t *testing.T) {
	user := &identity.User{ID: foundation.NewID(), Email: "old@example.com", Username: "old"}
	repository := &updateUserRepository{user: user, updateErr: identity.ErrUsernameAlreadyExists}
	newUsername := "taken"

	_, err := New(repository, time.Hour).UpdateUser(context.Background(), user.ID, nil, &newUsername)

	if !errors.Is(err, identity.ErrUsernameAlreadyExists) {
		t.Fatalf("expected ErrUsernameAlreadyExists, got %v", err)
	}
}

func (*missingUserRepository) FindUserByEmail(context.Context, string) (*identity.User, error) {
	return nil, sql.ErrNoRows
}

func TestLoginReturnsUnauthenticatedWhenUserDoesNotExist(t *testing.T) {
	service := New(&missingUserRepository{}, time.Hour)

	session, user, err := service.Login(context.Background(), "missing@example.com", "candidate-password")

	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
	if session != "" || user != nil {
		t.Fatalf("expected no authenticated session or user, got %q %#v", session, user)
	}
}

type viewerTokenRepository struct {
	identity.Repository
	user    *identity.User
	created *identity.APIToken
	touched foundation.ID
}

type serviceAccountTokenRepository struct {
	identity.Repository
	account     *identity.ServiceAccount
	activeCount int
	created     *identity.APIToken
}

func (r *serviceAccountTokenRepository) FindServiceAccountByID(_ context.Context, id foundation.ID) (*identity.ServiceAccount, error) {
	if r.account == nil || r.account.ID != id {
		return nil, sql.ErrNoRows
	}
	return r.account, nil
}

func (r *serviceAccountTokenRepository) CountActiveServiceAccountAPITokens(_ context.Context, serviceAccountID foundation.ID, _ time.Time) (int, error) {
	if r.account == nil || r.account.ID != serviceAccountID {
		return 0, sql.ErrNoRows
	}
	return r.activeCount, nil
}

func (r *serviceAccountTokenRepository) CreateServiceAccountAPIToken(_ context.Context, token *identity.APIToken, _ time.Time, maxActive int) error {
	if r.activeCount >= maxActive {
		return identity.ErrServiceAccountAccessKeyLimit
	}
	r.created = token
	return nil
}

func TestCreateServiceAccountAPITokenRejectsFourthActiveKey(t *testing.T) {
	account := &identity.ServiceAccount{ID: foundation.NewID()}
	repository := &serviceAccountTokenRepository{account: account, activeCount: constants.MaxActiveServiceAccountAccessKeys}

	created, err := New(repository, time.Hour).CreateServiceAccountAPIToken(context.Background(), account.ID, "pipeline", nil)

	if !errors.Is(err, identity.ErrServiceAccountAccessKeyLimit) {
		t.Fatalf("expected access-key limit error, got %v", err)
	}
	if created != nil || repository.created != nil {
		t.Fatalf("expected no key to be created, got %#v", created)
	}
}

func (r *viewerTokenRepository) FindUserByID(_ context.Context, id foundation.ID) (*identity.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, sql.ErrNoRows
	}
	return r.user, nil
}

func (r *viewerTokenRepository) CreateViewerAPIToken(_ context.Context, token *identity.APIToken) error {
	r.created = token
	return nil
}

func (r *viewerTokenRepository) FindAPITokenByPublicID(_ context.Context, publicID string) (*identity.APIToken, error) {
	if r.created == nil || r.created.PublicID != publicID {
		return nil, sql.ErrNoRows
	}
	return r.created, nil
}

func (r *viewerTokenRepository) TouchAPIToken(_ context.Context, id foundation.ID) error {
	r.touched = id
	return nil
}

func TestViewerRegistryTokenAuthenticatesOnlyItsViewer(t *testing.T) {
	viewer := &identity.User{ID: foundation.NewID(), Username: "viewer", SystemViewer: true}
	repository := &viewerTokenRepository{user: viewer}
	service := New(repository, time.Hour)

	created, err := service.CreateViewerRegistryToken(context.Background(), viewer.ID, "local pull", nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.created == nil || repository.created.Principal != (foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: viewer.ID}) {
		t.Fatalf("unexpected token principal: %#v", repository.created)
	}
	principal, err := service.AuthenticateRegistry(context.Background(), viewer.Username, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if principal != repository.created.Principal || repository.touched != repository.created.ID {
		t.Fatalf("unexpected authenticated principal %#v or touched token %q", principal, repository.touched)
	}
	if _, err := service.AuthenticateRegistry(context.Background(), "other", created.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected username mismatch to fail, got %v", err)
	}
}

func TestCreateViewerRegistryTokenRejectsRegularUser(t *testing.T) {
	user := &identity.User{ID: foundation.NewID(), Username: "member"}
	service := New(&viewerTokenRepository{user: user}, time.Hour)

	if _, err := service.CreateViewerRegistryToken(context.Background(), user.ID, "local pull", nil); err == nil {
		t.Fatal("expected regular user registry token creation to fail")
	}
}
