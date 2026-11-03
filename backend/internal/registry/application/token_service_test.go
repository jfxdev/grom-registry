package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
)

func TestScopeParametersAcceptTheSpaceDelimitedForm(t *testing.T) {
	// oras-go, and therefore ORAS, Helm and OpenTofu, joins several scopes into
	// one parameter separated by spaces whenever more than one repository is
	// involved. Docker sends one parameter per scope. Both must parse alike.
	joined := splitScopeParameters([]string{"repository:acme/modules:pull,push repository:acme/charts:pull"})
	repeated := splitScopeParameters([]string{"repository:acme/modules:pull,push", "repository:acme/charts:pull"})
	if len(joined) != len(repeated) {
		t.Fatalf("expected both scope forms to expand alike, got %#v and %#v", joined, repeated)
	}
	for index := range joined {
		if joined[index] != repeated[index] {
			t.Fatalf("expected scope %d to match, got %q and %q", index, joined[index], repeated[index])
		}
	}

	resourceType, name, actions, ok := parseScope(joined[0])
	if !ok || resourceType != "repository" || name != "acme/modules" {
		t.Fatalf("unexpected first scope: %q %q %t", resourceType, name, ok)
	}
	if len(actions) != 2 || actions[0] != "pull" || actions[1] != "push" {
		t.Fatalf("expected the first scope to keep pull and push, got %#v", actions)
	}
	if _, name, _, ok := parseScope(joined[1]); !ok || name != "acme/charts" {
		t.Fatalf("expected the second scope to survive, got %q %t", name, ok)
	}
}

func TestTokenServiceSubjectReturnsVerifiedRegistrySubject(t *testing.T) {
	signer, err := signing.LoadOrCreate(filepath.Join(t.TempDir(), "key.pem"), filepath.Join(t.TempDir(), "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewTokenService(nil, nil, signer, time.Minute)
	raw, err := service.IssueInternal("release-bot", nil)
	if err != nil {
		t.Fatal(err)
	}
	if subject, ok := service.Subject(raw); !ok || subject != "release-bot" {
		t.Fatalf("expected verified release-bot subject, got %q, %t", subject, ok)
	}
	if subject, ok := service.Subject("not-a-token"); ok || subject != "" {
		t.Fatalf("expected invalid token to have no subject, got %q, %t", subject, ok)
	}
}

type tokenProjectStore struct {
	projectdomain.Repository
	project    *projectdomain.Project
	membership *projectdomain.Membership
}

func (s *tokenProjectStore) FindProjectBySlug(context.Context, string) (*projectdomain.Project, error) {
	return s.project, nil
}

func (s *tokenProjectStore) FindMembership(context.Context, foundation.ID, foundation.PrincipalRef) (*projectdomain.Membership, error) {
	return s.membership, nil
}

func TestTokenServicePullThroughProjectsNeverGrantExternalPush(t *testing.T) {
	signer, err := signing.LoadOrCreate(filepath.Join(t.TempDir(), "key.pem"), filepath.Join(t.TempDir(), "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{constants.ProjectModeStandard, constants.ProjectModePullThrough} {
		for _, role := range []string{constants.RoleReader, constants.RoleWriter, constants.RoleAdmin} {
			t.Run(mode+"/"+role, func(t *testing.T) {
				store := &tokenProjectStore{
					project:    &projectdomain.Project{ID: "project", Slug: "mirror", Mode: mode},
					membership: &projectdomain.Membership{ProjectID: "project", Role: role},
				}
				service := NewTokenService(projectapp.New(store), nil, signer, time.Minute)
				raw, _, _, err := service.Issue(context.Background(), "registry-client", foundation.PrincipalRef{
					Kind: constants.PrincipalServiceAccount, ID: "account",
				}, "", []string{"repository:mirror/library/alpine:pull,push,delete"})
				if err != nil {
					t.Fatal(err)
				}
				if !service.HasAccess(raw, "mirror/library/alpine", constants.RegistryActionPull) {
					t.Fatal("project member did not receive pull")
				}
				if service.HasAccess(raw, "mirror/library/alpine", constants.RegistryActionDelete) {
					t.Fatal("external principal received delete")
				}
				push := service.HasAccess(raw, "mirror/library/alpine", constants.RegistryActionPush)
				expectedPush := mode == constants.ProjectModeStandard && role != constants.RoleReader
				if push != expectedPush {
					t.Fatalf("push access=%t, expected %t", push, expectedPush)
				}
			})
		}
	}
}
