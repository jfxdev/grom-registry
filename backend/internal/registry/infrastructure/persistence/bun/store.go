package bunstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

type Store struct {
	db *bun.DB
}

func New(db *bun.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateRepository(ctx context.Context, repository *registrydomain.Repository) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(fromRepository(repository)).Exec(ctx); err != nil {
			return err
		}
		for i := range repository.Policies {
			model, err := fromPolicy(&repository.Policies[i])
			if err != nil {
				return err
			}
			if _, err := tx.NewInsert().Model(model).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) EnsureRepository(
	ctx context.Context,
	repository *registrydomain.Repository,
) (*registrydomain.Repository, bool, error) {
	result, err := s.db.NewInsert().Model(fromRepository(repository)).
		On("CONFLICT (project_id, name) DO NOTHING").Exec(ctx)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows == 1 {
		return repository, true, nil
	}
	existing, err := s.FindRepository(ctx, repository.ProjectID, repository.Name)
	return existing, false, err
}

func (s *Store) ListRepositories(ctx context.Context, projectID foundation.ID) ([]registrydomain.Repository, error) {
	var models []repositoryModel
	if err := s.db.NewSelect().Model(&models).
		Where("project_id = ?", projectID.String()).
		OrderExpr("name ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]registrydomain.Repository, 0, len(models))
	for i := range models {
		repository := toRepository(&models[i])
		policies, err := s.listPolicies(ctx, repository.ID)
		if err != nil {
			return nil, err
		}
		repository.Policies = policies
		result = append(result, *repository)
	}
	return result, nil
}

func (s *Store) FindRepository(ctx context.Context, projectID foundation.ID, name string) (*registrydomain.Repository, error) {
	model := new(repositoryModel)
	if err := s.db.NewSelect().Model(model).
		Where("project_id = ?", projectID.String()).
		Where("name = ?", name).Scan(ctx); err != nil {
		return nil, err
	}
	repository := toRepository(model)
	policies, err := s.listPolicies(ctx, repository.ID)
	if err != nil {
		return nil, err
	}
	repository.Policies = policies
	return repository, nil
}

func (s *Store) FindRepositoryByID(ctx context.Context, repositoryID foundation.ID) (*registrydomain.Repository, error) {
	model := new(repositoryModel)
	if err := s.db.NewSelect().Model(model).Where("id = ?", repositoryID.String()).Scan(ctx); err != nil {
		return nil, err
	}
	repository := toRepository(model)
	policies, err := s.listPolicies(ctx, repository.ID)
	if err != nil {
		return nil, err
	}
	repository.Policies = policies
	return repository, nil
}

func (s *Store) RepositoryExists(ctx context.Context, projectID foundation.ID, name string) (bool, error) {
	return s.db.NewSelect().Model((*repositoryModel)(nil)).
		Where("project_id = ?", projectID.String()).
		Where("name = ?", name).Exists(ctx)
}

func (s *Store) ProjectHasRepositories(ctx context.Context, projectID foundation.ID) (bool, error) {
	return s.db.NewSelect().Model((*repositoryModel)(nil)).
		Where("project_id = ?", projectID.String()).Exists(ctx)
}

func (s *Store) UpsertDiscoveredRepository(ctx context.Context, repository *registrydomain.Repository) error {
	model := fromRepository(repository)
	_, err := s.db.NewInsert().Model(model).
		On("CONFLICT (project_id, name) DO UPDATE").
		Set("status = EXCLUDED.status").
		Set("updated_at = EXCLUDED.updated_at").Exec(ctx)
	return err
}

func (s *Store) SetRepositoryStatus(ctx context.Context, repositoryID foundation.ID, status string) error {
	_, err := s.db.NewUpdate().Model((*repositoryModel)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", repositoryID.String()).Exec(ctx)
	return err
}

func (s *Store) SaveRepositoryProfile(ctx context.Context, repository *registrydomain.Repository) error {
	_, err := s.db.NewUpdate().Model((*repositoryModel)(nil)).
		Set("profile = ?", repository.Profile).
		Set("profile_source = ?", repository.ProfileSource).
		Set("profile_confidence = ?", repository.ProfileConfidence).
		Set("profile_inferred_at = ?", repository.ProfileInferredAt).
		Set("profile_needs_review = ?", repository.ProfileNeedsReview).
		Set("updated_at = ?", repository.UpdatedAt).
		Where("id = ?", repository.ID.String()).Exec(ctx)
	return err
}

func (s *Store) ReplaceRepositoryPolicies(
	ctx context.Context,
	repositoryID foundation.ID,
	expectedVersion int,
	policies []registrydomain.Policy,
	updatedAt time.Time,
) (int, error) {
	nextVersion := expectedVersion + 1
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewUpdate().Model((*repositoryModel)(nil)).
			Set("policy_version = ?", nextVersion).
			Set("updated_at = ?", updatedAt).
			Where("id = ?", repositoryID.String()).
			Where("policy_version = ?", expectedVersion).Exec(ctx)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return fmt.Errorf("repository policies changed; reload before saving")
		}
		if _, err := tx.NewDelete().Model((*policyModel)(nil)).
			Where("repository_id = ?", repositoryID.String()).Exec(ctx); err != nil {
			return err
		}
		for i := range policies {
			model, err := fromPolicy(&policies[i])
			if err != nil {
				return err
			}
			if _, err := tx.NewInsert().Model(model).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	return nextVersion, err
}

func (s *Store) listPolicies(ctx context.Context, repositoryID foundation.ID) ([]registrydomain.Policy, error) {
	var models []policyModel
	if err := s.db.NewSelect().Model(&models).
		Where("repository_id = ?", repositoryID.String()).
		OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]registrydomain.Policy, 0, len(models))
	for i := range models {
		policy, err := toPolicy(&models[i])
		if err != nil {
			return nil, err
		}
		result = append(result, *policy)
	}
	return result, nil
}

func fromRepository(repository *registrydomain.Repository) *repositoryModel {
	if repository.Profile == "" {
		repository.Profile = constants.RepositoryProfileUnknown
	}
	if repository.ProfileSource == "" {
		repository.ProfileSource = constants.ProfileSourceNone
	}
	if repository.ProfileConfidence == "" {
		repository.ProfileConfidence = constants.ClassificationConfidenceNone
	}
	if repository.CreationSource == "" {
		repository.CreationSource = constants.RepositoryCreationManual
	}
	return &repositoryModel{
		ID: repository.ID.String(), ProjectID: repository.ProjectID.String(), Name: repository.Name,
		Description: repository.Description, Status: repository.Status, CreationSource: repository.CreationSource,
		Profile: repository.Profile, ProfileSource: repository.ProfileSource,
		ProfileConfidence: repository.ProfileConfidence, ProfileInferredAt: repository.ProfileInferredAt,
		ProfileNeedsReview: repository.ProfileNeedsReview,
		PolicyVersion:      repository.PolicyVersion,
		CreatedAt:          repository.CreatedAt, UpdatedAt: repository.UpdatedAt,
	}
}

func toRepository(model *repositoryModel) *registrydomain.Repository {
	return &registrydomain.Repository{
		ID: foundation.ID(model.ID), ProjectID: foundation.ID(model.ProjectID), Name: model.Name,
		Description: model.Description, Status: model.Status, CreationSource: model.CreationSource,
		Profile: model.Profile, ProfileSource: model.ProfileSource,
		ProfileConfidence: model.ProfileConfidence, ProfileInferredAt: model.ProfileInferredAt,
		ProfileNeedsReview: model.ProfileNeedsReview, PolicyVersion: model.PolicyVersion,
		Policies:  []registrydomain.Policy{},
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
	}
}

func fromPolicy(policy *registrydomain.Policy) (*policyModel, error) {
	configuration, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	return &policyModel{
		ID: policy.ID.String(), RepositoryID: policy.RepositoryID.String(), Type: policy.Type,
		ConfigurationJSON: string(configuration), Enabled: policy.Enabled, Version: policy.Version,
		CreatedAt: policy.CreatedAt, UpdatedAt: policy.UpdatedAt,
	}, nil
}

func toPolicy(model *policyModel) (*registrydomain.Policy, error) {
	var policy registrydomain.Policy
	if err := json.Unmarshal([]byte(model.ConfigurationJSON), &policy); err != nil {
		return nil, err
	}
	policy.ID = foundation.ID(model.ID)
	policy.RepositoryID = foundation.ID(model.RepositoryID)
	policy.Type = model.Type
	policy.Enabled = model.Enabled
	policy.Version = model.Version
	policy.CreatedAt = model.CreatedAt
	policy.UpdatedAt = model.UpdatedAt
	return &policy, nil
}

var _ registrydomain.Store = (*Store)(nil)
