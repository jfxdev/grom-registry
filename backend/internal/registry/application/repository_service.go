package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

var repositoryNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)

var ErrRepositoryExists = errors.New("repository already exists")
var ErrRepositoryArchived = errors.New("repository is archived")
var ErrRepositoryNotArchived = errors.New("repository must be archived before removal")
var ErrRepositoryNotEmpty = errors.New("repository still contains manifest inventory")

type RepositoryService struct {
	store         registrydomain.Store
	existingProbe func(context.Context, string) (bool, error)
	audit         AuditRecorder
}

func NewRepositoryService(store registrydomain.Store) *RepositoryService {
	return &RepositoryService{store: store}
}

func (s *RepositoryService) SetExistingRepositoryProbe(probe func(context.Context, string) (bool, error)) {
	s.existingProbe = probe
}

func (s *RepositoryService) SetAuditRecorder(audit AuditRecorder) {
	s.audit = audit
}

func (s *RepositoryService) Create(
	ctx context.Context,
	projectID foundation.ID,
	name string,
	description string,
	policies []registrydomain.Policy,
) (*registrydomain.Repository, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	description = strings.TrimSpace(description)
	if !repositoryNamePattern.MatchString(name) {
		return nil, fmt.Errorf("repository path must use lowercase path segments")
	}
	if len(name) > 255 || len(description) > 500 {
		return nil, fmt.Errorf("repository path or description is too long")
	}
	if len(policies) > 16 {
		return nil, fmt.Errorf("a repository can define at most 16 policies")
	}
	if exists, err := s.store.RepositoryExists(ctx, projectID, name); err != nil {
		return nil, err
	} else if exists {
		return nil, ErrRepositoryExists
	}

	now := time.Now().UTC()
	repositoryID := foundation.NewID()
	for i := range policies {
		if err := validatePolicy(&policies[i]); err != nil {
			return nil, fmt.Errorf("policy %d: %w", i+1, err)
		}
		policies[i].ID = foundation.NewID()
		policies[i].RepositoryID = repositoryID
		policies[i].Version = 1
		policies[i].CreatedAt = now
		policies[i].UpdatedAt = now
	}
	repository := &registrydomain.Repository{
		ID: repositoryID, ProjectID: projectID, Name: name, Description: description,
		Status: constants.RepositoryStatusEmpty, CreationSource: constants.RepositoryCreationManual,
		Profile:       constants.RepositoryProfileUnknown,
		ProfileSource: constants.ProfileSourceNone, ProfileConfidence: constants.ClassificationConfidenceNone,
		PolicyVersion: func() int {
			if len(policies) > 0 {
				return 1
			}
			return 0
		}(),
		Policies: policies, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateRepository(ctx, repository); err != nil {
		return nil, err
	}
	return repository, nil
}

func (s *RepositoryService) EnsureFromPush(
	ctx context.Context,
	projectID foundation.ID,
	name string,
	actor foundation.PrincipalRef,
) (*registrydomain.Repository, bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !repositoryNamePattern.MatchString(name) || len(name) > 255 {
		return nil, false, fmt.Errorf("repository path must use lowercase path segments")
	}
	now := time.Now().UTC()
	repository, created, err := s.store.EnsureRepository(ctx, &registrydomain.Repository{
		ID: foundation.NewID(), ProjectID: projectID, Name: name,
		Status: constants.RepositoryStatusEmpty, CreationSource: constants.RepositoryCreationPush,
		Profile: constants.RepositoryProfileUnknown, ProfileSource: constants.ProfileSourceNone,
		ProfileConfidence: constants.ClassificationConfidenceNone,
		Policies:          []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now,
	})
	if err == nil && !created && repository.Status == constants.RepositoryStatusArchived {
		return repository, false, ErrRepositoryArchived
	}
	if err != nil || !created || s.audit == nil {
		return repository, created, err
	}
	if err := s.audit.Record(ctx, actor, constants.AuditRepositoryCreatedFromPush,
		constants.AuditResourceRepository, repository.ID, map[string]any{
			"projectId": projectID,
			"name":      name,
			"source":    constants.RepositoryCreationPush,
		}); err != nil {
		return repository, true, err
	}
	return repository, true, nil
}

func (s *RepositoryService) List(
	ctx context.Context,
	projectID foundation.ID,
	discovered []string,
	catalogAvailable bool,
) ([]registrydomain.Repository, error) {
	if catalogAvailable {
		for _, rawName := range discovered {
			name := strings.TrimSpace(rawName)
			if !repositoryNamePattern.MatchString(name) {
				continue
			}
			now := time.Now().UTC()
			if err := s.store.UpsertDiscoveredRepository(ctx, &registrydomain.Repository{
				ID: foundation.NewID(), ProjectID: projectID, Name: name,
				Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationReconciled,
				Profile:       constants.RepositoryProfileUnknown,
				ProfileSource: constants.ProfileSourceNone, ProfileConfidence: constants.ClassificationConfidenceNone,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return nil, err
			}
		}
	}
	repositories, err := s.store.ListRepositories(ctx, projectID)
	if err != nil {
		return nil, err
	}
	active := make(map[string]struct{}, len(discovered))
	for _, name := range discovered {
		active[name] = struct{}{}
	}
	for i := range repositories {
		_, isActive := active[repositories[i].Name]
		expectedStatus := constants.RepositoryStatusEmpty
		if isActive {
			expectedStatus = constants.RepositoryStatusActive
		}
		if catalogAvailable && repositories[i].Status != constants.RepositoryStatusArchived && repositories[i].Status != expectedStatus {
			if err := s.store.SetRepositoryStatus(ctx, repositories[i].ID, expectedStatus); err != nil {
				return nil, err
			}
			repositories[i].Status = expectedStatus
		}
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i].Name < repositories[j].Name })
	return repositories, nil
}

func (s *RepositoryService) Find(ctx context.Context, projectID foundation.ID, name string) (*registrydomain.Repository, error) {
	return s.store.FindRepository(ctx, projectID, name)
}

func (s *RepositoryService) FindByID(ctx context.Context, repositoryID foundation.ID) (*registrydomain.Repository, error) {
	return s.store.FindRepositoryByID(ctx, repositoryID)
}

func (s *RepositoryService) ReplacePolicies(
	ctx context.Context,
	projectID, repositoryID foundation.ID,
	expectedVersion int,
	policies []registrydomain.Policy,
	actor foundation.PrincipalRef,
) (*registrydomain.Repository, error) {
	target, err := s.store.FindRepositoryByID(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if target.ProjectID != projectID {
		return nil, sql.ErrNoRows
	}
	if expectedVersion < 0 {
		return nil, fmt.Errorf("expected policy version must not be negative")
	}
	if len(policies) > 16 {
		return nil, fmt.Errorf("a repository can define at most 16 policies")
	}
	now := time.Now().UTC()
	nextVersion := expectedVersion + 1
	for i := range policies {
		if err := validatePolicy(&policies[i]); err != nil {
			return nil, fmt.Errorf("policy %d: %w", i+1, err)
		}
		policies[i].ID = foundation.NewID()
		policies[i].RepositoryID = target.ID
		policies[i].Version = nextVersion
		policies[i].CreatedAt = now
		policies[i].UpdatedAt = now
	}
	version, err := s.store.ReplaceRepositoryPolicies(ctx, target.ID, expectedVersion, policies, now)
	if err != nil {
		return nil, err
	}
	target.PolicyVersion = version
	target.Policies = policies
	target.UpdatedAt = now
	if s.audit != nil {
		if err := s.audit.Record(ctx, actor, constants.AuditRepositoryPoliciesUpdated,
			constants.AuditResourceRepository, target.ID, map[string]any{
				"previousVersion": expectedVersion,
				"version":         version,
				"policyCount":     len(policies),
			}); err != nil {
			return nil, err
		}
	}
	return target, nil
}

func (s *RepositoryService) Exists(ctx context.Context, projectID foundation.ID, name string) bool {
	exists, err := s.store.RepositoryExists(ctx, projectID, name)
	return err == nil && exists
}

func (s *RepositoryService) ProjectHasRepositories(ctx context.Context, projectID foundation.ID) (bool, error) {
	return s.store.ProjectHasRepositories(ctx, projectID)
}

func (s *RepositoryService) Archive(ctx context.Context, projectID, repositoryID foundation.ID, actor foundation.PrincipalRef) error {
	repository, err := s.store.FindRepositoryByID(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return err
	}
	if repository.ProjectID != projectID {
		return sql.ErrNoRows
	}
	if repository.Status == constants.RepositoryStatusArchived {
		return nil
	}
	if err := s.store.SetRepositoryStatus(ctx, repositoryID, constants.RepositoryStatusArchived); err != nil {
		return err
	}
	if s.audit != nil {
		return s.audit.Record(ctx, actor, constants.AuditRepositoryArchived, constants.AuditResourceRepository, repositoryID, map[string]any{"name": repository.Name})
	}
	return nil
}

func (s *RepositoryService) Remove(ctx context.Context, projectID, repositoryID foundation.ID, actor foundation.PrincipalRef) error {
	repository, err := s.store.FindRepositoryByID(ctx, repositoryID)
	if err != nil || repository.ProjectID != projectID {
		return sql.ErrNoRows
	}
	if repository.Status != constants.RepositoryStatusArchived {
		return ErrRepositoryNotArchived
	}
	hasLiveManifests, err := s.store.RepositoryHasLiveManifests(ctx, repositoryID)
	if err != nil {
		return err
	}
	if hasLiveManifests {
		return ErrRepositoryNotEmpty
	}
	if s.audit != nil {
		if err := s.audit.Record(ctx, actor, constants.AuditRepositoryRemoved, constants.AuditResourceRepository, repositoryID, map[string]any{"name": repository.Name}); err != nil {
			return err
		}
	}
	return s.store.DeleteRepository(ctx, repositoryID)
}

func (s *RepositoryService) CanReceivePush(
	ctx context.Context,
	projectID foundation.ID,
	projectSlug string,
	name string,
) bool {
	if repository, err := s.store.FindRepository(ctx, projectID, name); err == nil {
		return repository.Status != constants.RepositoryStatusArchived
	}
	if s.existingProbe == nil {
		return false
	}
	exists, err := s.existingProbe(ctx, projectSlug+"/"+name)
	if err != nil || !exists {
		return false
	}
	now := time.Now().UTC()
	err = s.store.UpsertDiscoveredRepository(ctx, &registrydomain.Repository{
		ID: foundation.NewID(), ProjectID: projectID, Name: name,
		Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationReconciled,
		Profile:       constants.RepositoryProfileUnknown,
		ProfileSource: constants.ProfileSourceNone, ProfileConfidence: constants.ClassificationConfidenceNone,
		CreatedAt: now, UpdatedAt: now,
	})
	return err == nil
}

func (s *RepositoryService) EvaluateTagMutation(ctx context.Context, projectID foundation.ID, repository, tag string, tagExists bool) error {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("repository must be created in Grom before pushing")
		}
		return err
	}
	for _, policy := range target.Policies {
		if !policy.Enabled || !matchesAnyTag(tag, policy.TagPatterns) {
			continue
		}
		if tagExists && policy.PreventOverwrite &&
			(policy.Type == constants.RepositoryPolicyTagProtection || policy.Type == constants.RepositoryPolicyImmutability) {
			return fmt.Errorf("tag %q is immutable for this repository", tag)
		}
		if policy.Type == constants.RepositoryPolicyTagNaming && len(policy.AllowedPatterns) > 0 && !matchesAnyTag(tag, policy.AllowedPatterns) {
			return fmt.Errorf("tag %q does not match this repository's naming policy", tag)
		}
	}
	return nil
}

func (s *RepositoryService) EvaluateDeletion(
	ctx context.Context,
	projectID foundation.ID,
	repository string,
	tags []string,
	reason string,
	enforceReason bool,
) (bool, error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return false, err
	}
	requiresReason := false
	for _, policy := range target.Policies {
		if policy.Enabled && policy.Type == constants.RepositoryPolicyManualDeletion && policy.RequireReason {
			requiresReason = true
		}
	}
	for _, policy := range target.Policies {
		if !policy.Enabled {
			continue
		}
		if policy.Type != constants.RepositoryPolicyTagProtection || !policy.PreventDeletion {
			continue
		}
		for _, tag := range tags {
			if matchesAnyTag(tag, policy.TagPatterns) {
				return requiresReason, fmt.Errorf("tag %q is protected from deletion", tag)
			}
		}
	}
	if enforceReason && requiresReason && strings.TrimSpace(reason) == "" {
		return true, fmt.Errorf("a deletion reason is required by repository policy")
	}
	return requiresReason, nil
}

func validatePolicy(policy *registrydomain.Policy) error {
	switch policy.Type {
	case constants.RepositoryPolicyTagProtection:
		if len(policy.TagPatterns) == 0 {
			return fmt.Errorf("protected tag patterns are required")
		}
	case constants.RepositoryPolicyImmutability:
		if len(policy.TagPatterns) == 0 || !policy.PreventOverwrite {
			return fmt.Errorf("immutable tag patterns and overwrite protection are required")
		}
	case constants.RepositoryPolicyRetention:
		if policy.ExpireAfterDays == nil && policy.KeepLast == nil && policy.UntaggedGraceDays == nil {
			return fmt.Errorf("at least one retention limit is required")
		}
	case constants.RepositoryPolicyTagNaming:
		if len(policy.AllowedPatterns) == 0 {
			return fmt.Errorf("allowed tag patterns are required")
		}
	case constants.RepositoryPolicyManualDeletion:
	default:
		return fmt.Errorf("unknown policy type %q", policy.Type)
	}
	if len(policy.TagPatterns) > 32 || len(policy.AllowedPatterns) > 32 {
		return fmt.Errorf("a policy can define at most 32 patterns")
	}
	for _, pattern := range append(append([]string{}, policy.TagPatterns...), policy.AllowedPatterns...) {
		if pattern == "" || len(pattern) > 128 || strings.Contains(pattern, "/") {
			return fmt.Errorf("tag pattern %q is invalid", pattern)
		}
		if _, err := path.Match(pattern, "example"); err != nil {
			return fmt.Errorf("tag pattern %q is invalid", pattern)
		}
	}
	if !within(policy.ExpireAfterDays, 1, 3650) || !within(policy.UntaggedGraceDays, 1, 3650) || !within(policy.KeepLast, 1, 10000) {
		return fmt.Errorf("policy numeric limits are outside the supported range")
	}
	return nil
}

func within(value *int, minimum, maximum int) bool {
	return value == nil || (*value >= minimum && *value <= maximum)
}

func matchesAnyTag(tag string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if matched, _ := path.Match(pattern, tag); matched {
			return true
		}
	}
	return false
}
