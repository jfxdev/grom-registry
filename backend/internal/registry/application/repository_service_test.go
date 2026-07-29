package application

import (
	"context"
	"testing"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type deletionPolicyStore struct {
	registrydomain.Store
	repository *registrydomain.Repository
}

func (s *deletionPolicyStore) FindRepository(
	context.Context,
	foundation.ID,
	string,
) (*registrydomain.Repository, error) {
	return s.repository, nil
}

func TestEvaluateDeletionDeterminesReasonRequirementBeforeProtection(t *testing.T) {
	service := NewRepositoryService(&deletionPolicyStore{repository: &registrydomain.Repository{
		Policies: []registrydomain.Policy{{
			Type: constants.RepositoryPolicyTagProtection, Enabled: true,
			TagPatterns: []string{"prod"}, PreventDeletion: true,
		}, {
			Type: constants.RepositoryPolicyManualDeletion, Enabled: true, RequireReason: true,
		}},
	}})

	requiresReason, err := service.EvaluateDeletion(
		context.Background(), foundation.ID("project"), "api", []string{"prod"}, "", true,
	)
	if err == nil {
		t.Fatal("expected protected tag deletion to be rejected")
	}
	if !requiresReason {
		t.Fatal("expected reason requirement from a later policy to be preserved")
	}
}
