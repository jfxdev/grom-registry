package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
)

type Access struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type RegistryClaims struct {
	Access []Access `json:"access"`
	jwt.RegisteredClaims
}

type TokenService struct {
	projects     *projectapp.Service
	repositories *RepositoryService
	signer       *signing.Signer
	issuer       string
	service      string
	ttl          time.Duration
}

func NewTokenService(projects *projectapp.Service, repositories *RepositoryService, signer *signing.Signer, ttl time.Duration) *TokenService {
	return &TokenService{
		projects: projects, repositories: repositories, signer: signer, issuer: constants.RegistryIssuer,
		service: constants.RegistryService, ttl: ttl,
	}
}

func (s *TokenService) Issue(ctx context.Context, subject string, principal foundation.PrincipalRef, service string, scopes []string) (string, int, time.Time, error) {
	if service == "" {
		service = s.service
	}
	access := make([]Access, 0, len(scopes))
	for _, raw := range scopes {
		resourceType, name, actions, ok := parseScope(raw)
		if !ok || resourceType != "repository" {
			continue
		}
		allowed := s.projects.AllowedActions(ctx, principal, name, actions)
		if principal.Kind == constants.PrincipalUser {
			allowed = onlyPullActions(allowed)
		}
		if containsAction(allowed, constants.RegistryActionPush) && !s.repositoryCanReceivePush(ctx, name) {
			if err := s.ensureRepositoryForPush(ctx, name, principal); err != nil {
				if errors.Is(err, ErrRepositoryArchived) {
					allowed = withoutAction(allowed, constants.RegistryActionPush)
					access = append(access, Access{Type: resourceType, Name: name, Actions: allowed})
					continue
				}
				return "", 0, time.Time{}, err
			}
		}
		access = append(access, Access{Type: resourceType, Name: name, Actions: allowed})
	}
	now := time.Now().UTC()
	expires := now.Add(s.ttl)
	claims := RegistryClaims{
		Access: access,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.issuer, Subject: subject, Audience: jwt.ClaimStrings{service},
			ExpiresAt: jwt.NewNumericDate(expires), NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			IssuedAt: jwt.NewNumericDate(now), ID: uuid.NewString(),
		},
	}
	token, err := s.signer.Sign(claims)
	return token, int(s.ttl.Seconds()), now, err
}

func onlyPullActions(actions []string) []string {
	result := make([]string, 0, 1)
	for _, action := range actions {
		if action == constants.RegistryActionPull {
			result = append(result, action)
		}
	}
	return result
}

func withoutAction(actions []string, denied string) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		if action != denied {
			result = append(result, action)
		}
	}
	return result
}

func (s *TokenService) ensureRepositoryForPush(
	ctx context.Context,
	fullRepository string,
	principal foundation.PrincipalRef,
) error {
	if s.repositories == nil {
		return nil
	}
	projectSlug, repositoryName, ok := strings.Cut(fullRepository, "/")
	if !ok || repositoryName == "" {
		return fmt.Errorf("repository path is invalid")
	}
	project, err := s.projects.Find(ctx, projectSlug)
	if err != nil {
		return err
	}
	_, _, err = s.repositories.EnsureFromPush(ctx, project.ID, repositoryName, principal)
	return err
}

func (s *TokenService) repositoryCanReceivePush(ctx context.Context, fullName string) bool {
	if s.repositories == nil {
		return true
	}
	projectSlug, repositoryName, ok := strings.Cut(fullName, "/")
	if !ok || repositoryName == "" {
		return false
	}
	project, err := s.projects.Find(ctx, projectSlug)
	if err != nil {
		return false
	}
	return s.repositories.CanReceivePush(ctx, project.ID, projectSlug, repositoryName)
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func (s *TokenService) IssueCatalog(subject string) (string, error) {
	return s.IssueInternal(subject, []Access{{Type: "registry", Name: "catalog", Actions: []string{"*"}}})
}

func (s *TokenService) IssueInternal(subject string, access []Access) (string, error) {
	now := time.Now().UTC()
	claims := RegistryClaims{
		Access: access,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.issuer, Subject: subject, Audience: jwt.ClaimStrings{s.service},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)), NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			IssuedAt: jwt.NewNumericDate(now), ID: uuid.NewString(),
		},
	}
	return s.signer.Sign(claims)
}

func (s *TokenService) HasAccess(rawToken, repository, action string) bool {
	var claims RegistryClaims
	if err := s.signer.Verify(rawToken, &claims, s.issuer, s.service); err != nil {
		return false
	}
	return claimsHasAccess(&claims, repository, action)
}

func (s *TokenService) Subject(rawToken string) (string, bool) {
	var claims RegistryClaims
	if err := s.signer.Verify(rawToken, &claims, s.issuer, s.service); err != nil {
		return "", false
	}
	return claims.Subject, true
}

func claimsHasAccess(claims *RegistryClaims, repository, action string) bool {
	for _, access := range claims.Access {
		if access.Type == "repository" && access.Name == repository && containsAction(access.Actions, action) {
			return true
		}
	}
	return false
}

func (s *TokenService) EvaluateManifestPut(ctx context.Context, fullRepository, tag string, tagExists bool) error {
	if s.repositories == nil {
		return nil
	}
	projectSlug, repositoryName, ok := strings.Cut(fullRepository, "/")
	if !ok || repositoryName == "" {
		return fmt.Errorf("repository path is invalid")
	}
	project, err := s.projects.Find(ctx, projectSlug)
	if err != nil {
		return fmt.Errorf("project not found")
	}
	return s.repositories.EvaluateTagMutation(ctx, project.ID, repositoryName, tag, tagExists)
}

func parseScope(raw string) (string, string, []string, bool) {
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", nil, false
	}
	actions := strings.Split(parts[2], ",")
	return parts[0], parts[1], actions, true
}

func ParseRepositoryProject(repository string) (string, error) {
	project, _, ok := strings.Cut(repository, "/")
	if !ok || project == "" {
		return "", fmt.Errorf("repository must start with a project slug")
	}
	return project, nil
}
