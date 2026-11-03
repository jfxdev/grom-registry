package application

import (
	"context"
	"slices"
	"testing"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
)

type modeProjectStore struct {
	projectdomain.Repository
	created    *projectdomain.Project
	membership *projectdomain.Membership
	project    *projectdomain.Project
	role       string
}

func (s *modeProjectStore) CreateProject(_ context.Context, project *projectdomain.Project, membership *projectdomain.Membership) error {
	s.created = project
	s.membership = membership
	return nil
}

func (s *modeProjectStore) FindProjectBySlug(context.Context, string) (*projectdomain.Project, error) {
	return s.project, nil
}

func (s *modeProjectStore) FindMembership(context.Context, foundation.ID, foundation.PrincipalRef) (*projectdomain.Membership, error) {
	return &projectdomain.Membership{Role: s.role}, nil
}

func TestCreateValidatesProjectModeAndUpstream(t *testing.T) {
	actor := foundation.PrincipalRef{Kind: "user", ID: foundation.ID("admin")}
	for _, test := range []struct {
		name         string
		mode         string
		upstream     string
		wantMode     string
		wantUpstream string
		wantErr      bool
	}{
		{name: "default standard", mode: " ", wantMode: constants.ProjectModeStandard},
		{name: "standard with upstream", mode: constants.ProjectModeStandard, upstream: constants.ProjectUpstreamGHCR, wantErr: true},
		{name: "pull-through dockerhub", mode: constants.ProjectModePullThrough, upstream: " dockerhub ", wantMode: constants.ProjectModePullThrough, wantUpstream: constants.ProjectUpstreamDockerHub},
		{name: "pull-through ghcr", mode: constants.ProjectModePullThrough, upstream: constants.ProjectUpstreamGHCR, wantMode: constants.ProjectModePullThrough, wantUpstream: constants.ProjectUpstreamGHCR},
		{name: "pull-through without upstream", mode: constants.ProjectModePullThrough, wantErr: true},
		{name: "pull-through with unknown upstream", mode: constants.ProjectModePullThrough, upstream: "quay", wantErr: true},
		{name: "unknown mode", mode: "mirror", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &modeProjectStore{}
			project, err := New(store).Create(context.Background(), actor, true, "Mirror", "mirror", test.mode, test.upstream)
			if test.wantErr {
				if err == nil || store.created != nil {
					t.Fatalf("invalid project was accepted: project=%#v err=%v", project, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if project.Mode != test.wantMode || project.Upstream != test.wantUpstream || store.created != project {
				t.Fatalf("unexpected project: %#v", project)
			}
			if store.membership.Role != constants.RoleAdmin || store.membership.PrincipalID != actor.ID {
				t.Fatalf("creator was not made project administrator: %#v", store.membership)
			}
		})
	}
}

func TestCreateRequiresAdministrator(t *testing.T) {
	store := &modeProjectStore{}
	if _, err := New(store).Create(context.Background(), foundation.PrincipalRef{}, false, "Mirror", "mirror", "", ""); err == nil || store.created != nil {
		t.Fatalf("non-administrator created a project: err=%v", err)
	}
}

func TestAllowedActionsNeverGrantsPushInPullThroughProjects(t *testing.T) {
	principal := foundation.PrincipalRef{Kind: "service_account", ID: foundation.ID("writer")}
	requested := []string{constants.RegistryActionPull, constants.RegistryActionPush, "delete"}
	for _, test := range []struct {
		mode string
		role string
		want []string
	}{
		{mode: constants.ProjectModeStandard, role: constants.RoleWriter, want: []string{constants.RegistryActionPull, constants.RegistryActionPush}},
		{mode: constants.ProjectModeStandard, role: constants.RoleReader, want: []string{constants.RegistryActionPull}},
		{mode: constants.ProjectModePullThrough, role: constants.RoleWriter, want: []string{constants.RegistryActionPull}},
		{mode: constants.ProjectModePullThrough, role: constants.RoleAdmin, want: []string{constants.RegistryActionPull}},
	} {
		t.Run(test.mode+"/"+test.role, func(t *testing.T) {
			store := &modeProjectStore{project: &projectdomain.Project{ID: foundation.ID("project"), Slug: "mirror", Mode: test.mode}, role: test.role}
			got := New(store).AllowedActions(context.Background(), principal, "mirror/alpine", requested)
			if !slices.Equal(got, test.want) {
				t.Fatalf("allowed actions = %v, want %v", got, test.want)
			}
		})
	}
}
