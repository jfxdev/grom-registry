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
