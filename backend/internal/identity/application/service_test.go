package application

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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
