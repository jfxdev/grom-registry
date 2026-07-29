package application

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
)

type missingUserRepository struct {
	identity.Repository
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
