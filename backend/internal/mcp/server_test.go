package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

const (
	subjectDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	sbomDigest    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	missingDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

var (
	reader       = foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: "reader"}
	admin        = foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: "admin"}
	viewerAdmin  = foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: "viewer"}
	outsider     = foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: "outsider"}
	alphaProject = projectdomain.Project{ID: "alpha-id", Slug: "alpha", Name: "Alpha", Mode: constants.ProjectModeStandard}
	betaProject  = projectdomain.Project{ID: "beta-id", Slug: "beta", Name: "Beta", Mode: constants.ProjectModeStandard}
)

type fakeProjects struct {
	projects []projectdomain.Project
	roles    map[foundation.ID]map[foundation.PrincipalRef]string
}

func (f *fakeProjects) ListPage(_ context.Context, actor foundation.PrincipalRef, systemAdmin bool, request foundation.PageRequest) (foundation.PageResult[projectdomain.Project], error) {
	if systemAdmin {
		panic("access-key principals must never be listed as system administrators")
	}
	if !strings.HasPrefix(request.Scope, "mcp:") {
		panic("cursor scope must be MCP-specific")
	}
	result := foundation.PageResult[projectdomain.Project]{Items: []projectdomain.Project{}}
	for _, project := range f.projects {
		if _, ok := f.roles[project.ID][actor]; ok {
			result.Items = append(result.Items, project)
		}
	}
	return result, nil
}

func (f *fakeProjects) Find(_ context.Context, slug string) (*projectdomain.Project, error) {
	for _, project := range f.projects {
		if project.Slug == slug {
			copied := project
			return &copied, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeProjects) CanView(_ context.Context, actor foundation.PrincipalRef, systemAdmin bool, project *projectdomain.Project) bool {
	_, ok := f.roles[project.ID][actor]
	return !systemAdmin && ok
}

func (f *fakeProjects) CanManage(_ context.Context, actor foundation.PrincipalRef, systemAdmin bool, project *projectdomain.Project) bool {
	return !systemAdmin && f.roles[project.ID][actor] == constants.RoleAdmin
}

type fakeRepositories struct {
	repositories []registrydomain.Repository
	tags         []string
}

func (f *fakeRepositories) ListPage(_ context.Context, projectID foundation.ID, query string, _ foundation.PageRequest) (foundation.PageResult[registrydomain.Repository], error) {
	result := foundation.PageResult[registrydomain.Repository]{Items: []registrydomain.Repository{}}
	for _, repository := range f.repositories {
		if repository.ProjectID == projectID && strings.Contains(repository.Name, query) {
			result.Items = append(result.Items, repository)
		}
	}
	return result, nil
}

func (f *fakeRepositories) Find(_ context.Context, projectID foundation.ID, name string) (*registrydomain.Repository, error) {
	for _, repository := range f.repositories {
		if repository.ProjectID == projectID && repository.Name == name {
			copied := repository
			return &copied, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeRepositories) FindByID(_ context.Context, id foundation.ID) (*registrydomain.Repository, error) {
	for _, repository := range f.repositories {
		if repository.ID == id {
			copied := repository
			return &copied, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeRepositories) SearchTagNames(_ context.Context, _ foundation.ID, query string, sort registrydomain.TagSort, request foundation.PageRequest) (foundation.PageResult[string], error) {
	result := foundation.PageResult[string]{Items: []string{}}
	for _, tag := range f.tags {
		if strings.Contains(tag, query) {
			result.Items = append(result.Items, tag)
		}
	}
	if sort == registrydomain.TagSortNameDesc {
		slices.Reverse(result.Items)
	}
	offset := 0
	if request.Cursor != "" {
		cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
		if err != nil {
			return foundation.PageResult[string]{}, err
		}
		offset = cursor.Offset
	}
	result.Items = result.Items[min(offset, len(result.Items)):]
	if len(result.Items) > request.Limit {
		result.Items = result.Items[:request.Limit]
		result.NextCursor, _ = foundation.EncodePageCursor(foundation.PageCursor{Scope: request.Scope, Offset: offset + request.Limit})
	}
	return result, nil
}

type fakeInventory struct {
	repositories *fakeRepositories
	items        []registrydomain.ManifestInventory
}

func (f *fakeInventory) repositoryID(projectID foundation.ID, name string) (foundation.ID, error) {
	repository, err := f.repositories.Find(context.Background(), projectID, name)
	if err != nil {
		return "", err
	}
	return repository.ID, nil
}

func (f *fakeInventory) ListPage(_ context.Context, projectID foundation.ID, repository, query string, _ foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	id, err := f.repositoryID(projectID, repository)
	if err != nil {
		return foundation.PageResult[registrydomain.ManifestInventory]{}, err
	}
	result := foundation.PageResult[registrydomain.ManifestInventory]{Items: []registrydomain.ManifestInventory{}}
	for _, item := range f.items {
		if item.RepositoryID == id && (query == "" || strings.Contains(item.Digest, query) || slices.Contains(item.Tags, query)) {
			result.Items = append(result.Items, item)
		}
	}
	return result, nil
}

func (f *fakeInventory) Find(_ context.Context, projectID foundation.ID, repository, digest string) (*registrydomain.ManifestInventory, error) {
	id, err := f.repositoryID(projectID, repository)
	if err != nil {
		return nil, err
	}
	for _, item := range f.items {
		if item.RepositoryID == id && item.Digest == digest {
			copied := item
			return &copied, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeInventory) ListReferrersPage(_ context.Context, projectID foundation.ID, repository, subject string, _ foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	id, err := f.repositoryID(projectID, repository)
	if err != nil {
		return foundation.PageResult[registrydomain.ManifestInventory]{}, err
	}
	result := foundation.PageResult[registrydomain.ManifestInventory]{Items: []registrydomain.ManifestInventory{}}
	for _, item := range f.items {
		if item.RepositoryID == id && item.SubjectDigest == subject {
			result.Items = append(result.Items, item)
		}
	}
	return result, nil
}

func (f *fakeInventory) ProjectUsage(context.Context, foundation.ID) foundation.AccountedStorageUsage {
	bytes := int64(4096)
	return foundation.AccountedStorageUsage{Status: "reconciled", AccountedBytes: &bytes}
}

func (f *fakeInventory) ProjectUsages(ctx context.Context, ids []foundation.ID) map[foundation.ID]foundation.AccountedStorageUsage {
	result := map[foundation.ID]foundation.AccountedStorageUsage{}
	for _, id := range ids {
		result[id] = f.ProjectUsage(ctx, id)
	}
	return result
}

type fakeLifecycle struct {
	runs []registrydomain.LifecycleRun
}

func (f *fakeLifecycle) ListRunsPage(context.Context, foundation.ID, string, foundation.PageRequest) (foundation.PageResult[registrydomain.LifecycleRun], error) {
	return foundation.PageResult[registrydomain.LifecycleRun]{Items: f.runs[:1]}, nil
}

func (f *fakeLifecycle) FindRun(_ context.Context, id foundation.ID) (*registrydomain.LifecycleRun, error) {
	for _, run := range f.runs {
		if run.ID == id {
			copied := run
			return &copied, nil
		}
	}
	return nil, sql.ErrNoRows
}

type fakeDeletions struct{}

func (fakeDeletions) ListPage(context.Context, foundation.ID, string, foundation.PageRequest) (foundation.PageResult[registrydomain.ArtifactDeletion], error) {
	return foundation.PageResult[registrydomain.ArtifactDeletion]{Items: []registrydomain.ArtifactDeletion{{
		ID: foundation.NewID(), Digest: subjectDigest, ActorID: "admin", Status: "completed", StartedAt: time.Now().UTC(),
	}}}, nil
}

const (
	alphaRunID = "11111111-1111-4111-8111-111111111111"
	betaRunID  = "22222222-2222-4222-8222-222222222222"
)

func newTestDeps() Deps {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	repositories := &fakeRepositories{
		repositories: []registrydomain.Repository{
			{ID: "api-id", ProjectID: alphaProject.ID, Name: "api", Status: constants.RepositoryStatusActive, Profile: "container_image",
				PolicyVersion: 3, Policies: []registrydomain.Policy{{Type: constants.RepositoryPolicyTagProtection, Enabled: true}}},
			{ID: "secret-id", ProjectID: betaProject.ID, Name: "secret", Status: constants.RepositoryStatusActive},
		},
		tags: []string{"v1", "v2", "v3"},
	}
	return Deps{
		Projects: &fakeProjects{
			projects: []projectdomain.Project{alphaProject, betaProject},
			roles: map[foundation.ID]map[foundation.PrincipalRef]string{
				alphaProject.ID: {reader: constants.RoleReader, admin: constants.RoleAdmin, viewerAdmin: constants.RoleAdmin},
				betaProject.ID:  {outsider: constants.RoleAdmin},
			},
		},
		Repositories: repositories,
		Inventory: &fakeInventory{repositories: repositories, items: []registrydomain.ManifestInventory{
			{RepositoryID: "api-id", Digest: subjectDigest, ArtifactRelationship: constants.ArtifactRelationshipPrimary, Tags: []string{"v1"}, FirstSeenAt: now, LastSeenAt: now,
				Platforms: []registrydomain.ManifestPlatform{{OS: "linux", Architecture: "amd64", Digest: subjectDigest, CompressedSize: 10}}},
			{RepositoryID: "api-id", Digest: sbomDigest, SubjectDigest: subjectDigest, ArtifactType: "application/spdx+json", ArtifactRelationship: constants.ArtifactRelationshipReferrer, FirstSeenAt: now, LastSeenAt: now},
		}},
		Lifecycle: &fakeLifecycle{runs: []registrydomain.LifecycleRun{
			{ID: alphaRunID, RepositoryID: "api-id", Status: "completed", StartedAt: now, Items: []registrydomain.LifecycleRunItem{{Digest: subjectDigest, Status: "deleted"}}},
			{ID: betaRunID, RepositoryID: "secret-id", Status: "completed", StartedAt: now},
		}},
		Deletions:     fakeDeletions{},
		PolicyPresets: registryapp.PolicyPresets,
	}
}

func connect(t *testing.T, deps Deps, principal *foundation.PrincipalRef) *mcpsdk.ClientSession {
	t.Helper()
	handler := NewHandler(deps, "test")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(WithPrincipal(r.Context(), *principal))
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "grom-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{Endpoint: server.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, session *mcpsdk.ClientSession, name string, arguments map[string]any, output any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: unexpected tool error %s", name, toolText(result))
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, output); err != nil {
		t.Fatalf("%s: decode %s: %v", name, raw, err)
	}
}

// callError returns the error text a client sees, whether the SDK rejected
// the arguments as a protocol error or the tool returned an error result.
func callError(t *testing.T, session *mcpsdk.ClientSession, name string, arguments map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return err.Error()
	}
	if !result.IsError {
		t.Fatalf("%s(%v): expected an error, got %#v", name, arguments, result.StructuredContent)
	}
	return toolText(result)
}

func toolText(result *mcpsdk.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcpsdk.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func TestToolsAreExactlyTheReadOnlyMetadataSet(t *testing.T) {
	session := connect(t, newTestDeps(), &reader)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			t.Errorf("%s must be annotated read-only", tool.Name)
		}
	}
	slices.Sort(names)
	want := []string{
		"get_artifact", "get_lifecycle_run", "get_project", "get_repository", "list_artifact_deletions",
		"list_artifacts", "list_lifecycle_runs", "list_policy_presets", "list_projects", "list_referrers",
		"list_repositories", "list_tags",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "get_repository" {
			continue
		}
		schema, _ := json.Marshal(tool.OutputSchema)
		if strings.Contains(string(schema), "polic") {
			t.Fatalf("repository output schema exposes policies: %s", schema)
		}
	}
}

func TestProjectsAreLimitedToExplicitMembership(t *testing.T) {
	session := connect(t, newTestDeps(), &reader)

	var projects ListResult[Project]
	call(t, session, "list_projects", nil, &projects)
	if len(projects.Items) != 1 || projects.Items[0].Slug != "alpha" || projects.Items[0].AccountedUsage.Status != "reconciled" {
		t.Fatalf("projects = %#v", projects)
	}
	var project ProjectDetail
	call(t, session, "get_project", map[string]any{"project": "alpha"}, &project)
	if project.Slug != "alpha" || project.CanManage {
		t.Fatalf("project = %#v", project)
	}

	hidden := callError(t, session, "get_project", map[string]any{"project": "beta"})
	missing := callError(t, session, "get_project", map[string]any{"project": "nowhere"})
	if hidden != missing || !strings.Contains(hidden, errProjectNotFound.Error()) {
		t.Fatalf("hidden project error %q must equal missing project error %q", hidden, missing)
	}
	if text := callError(t, session, "get_repository", map[string]any{"project": "beta", "repository": "secret"}); text != missing {
		t.Fatalf("hidden repository leaked: %q", text)
	}
}

func TestRepositoryAndArtifactMetadata(t *testing.T) {
	session := connect(t, newTestDeps(), &reader)

	var repositories ListResult[Repository]
	call(t, session, "list_repositories", map[string]any{"project": "alpha"}, &repositories)
	if len(repositories.Items) != 1 || repositories.Items[0].Name != "api" {
		t.Fatalf("repositories = %#v", repositories)
	}
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "get_repository", Arguments: map[string]any{"project": "alpha", "repository": "api"}})
	if err != nil || result.IsError {
		t.Fatalf("get_repository: %v %s", err, toolText(result))
	}
	raw, _ := json.Marshal(result.StructuredContent)
	if strings.Contains(string(raw), "polic") || !strings.Contains(string(raw), `"profile":"container_image"`) {
		t.Fatalf("repository payload = %s", raw)
	}

	var tags ListResult[string]
	call(t, session, "list_tags", map[string]any{"project": "alpha", "repository": "api", "sort": "name-desc", "limit": 2}, &tags)
	if !slices.Equal(tags.Items, []string{"v3", "v2"}) || tags.NextCursor == "" {
		t.Fatalf("tags = %#v", tags)
	}
	var lastTags ListResult[string]
	call(t, session, "list_tags", map[string]any{"project": "alpha", "repository": "api", "sort": "name-desc", "limit": 2, "cursor": tags.NextCursor}, &lastTags)
	if !slices.Equal(lastTags.Items, []string{"v1"}) || lastTags.NextCursor != "" {
		t.Fatalf("continuation page = %#v", lastTags)
	}
	if text := callError(t, session, "list_tags", map[string]any{"project": "alpha", "repository": "api", "sort": "oldest", "cursor": tags.NextCursor}); !strings.Contains(text, errInvalidPageRequest.Error()) {
		t.Fatalf("cursor from other arguments accepted: %q", text)
	}

	var artifacts ListResult[Artifact]
	call(t, session, "list_artifacts", map[string]any{"project": "alpha", "repository": "api"}, &artifacts)
	if len(artifacts.Items) != 2 {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	var artifact Artifact
	call(t, session, "get_artifact", map[string]any{"project": "alpha", "repository": "api", "digest": subjectDigest}, &artifact)
	if artifact.Digest != subjectDigest || len(artifact.Platforms) != 1 || !slices.Equal(artifact.Tags, []string{"v1"}) {
		t.Fatalf("artifact = %#v", artifact)
	}
	if text := callError(t, session, "get_artifact", map[string]any{"project": "alpha", "repository": "api", "digest": missingDigest}); !strings.Contains(text, errArtifactNotFound.Error()) {
		t.Fatalf("missing artifact error = %q", text)
	}
	var referrers ReferrersResult
	call(t, session, "list_referrers", map[string]any{"project": "alpha", "repository": "api", "digest": subjectDigest}, &referrers)
	if referrers.SubjectDigest != subjectDigest || len(referrers.Items) != 1 || referrers.Items[0].Digest != sbomDigest {
		t.Fatalf("referrers = %#v", referrers)
	}
	var presets ListResult[PolicyPreset]
	call(t, session, "list_policy_presets", nil, &presets)
	if len(presets.Items) == 0 {
		t.Fatal("expected policy presets")
	}
}

func TestHistoryRequiresProjectAdminMembership(t *testing.T) {
	deps := newTestDeps()
	readerSession := connect(t, deps, &reader)
	missing := callError(t, readerSession, "get_project", map[string]any{"project": "nowhere"})
	for name, arguments := range map[string]map[string]any{
		"list_lifecycle_runs":     {"project": "alpha", "repository": "api"},
		"get_lifecycle_run":       {"project": "alpha", "runId": alphaRunID},
		"list_artifact_deletions": {"project": "alpha", "repository": "api"},
	} {
		if text := callError(t, readerSession, name, arguments); text != missing {
			t.Errorf("%s for a Reader = %q, want %q", name, text, missing)
		}
	}

	for _, principal := range []*foundation.PrincipalRef{&admin, &viewerAdmin} {
		session := connect(t, deps, principal)
		var runs ListResult[LifecycleRun]
		call(t, session, "list_lifecycle_runs", map[string]any{"project": "alpha", "repository": "api"}, &runs)
		if len(runs.Items) != 1 {
			t.Fatalf("runs = %#v", runs)
		}
		var run LifecycleRun
		call(t, session, "get_lifecycle_run", map[string]any{"project": "alpha", "runId": alphaRunID}, &run)
		if run.Repository != "api" || len(run.Items) != 1 {
			t.Fatalf("run = %#v", run)
		}
		if text := callError(t, session, "get_lifecycle_run", map[string]any{"project": "alpha", "runId": betaRunID}); !strings.Contains(text, errRunNotFound.Error()) {
			t.Fatalf("another project's run leaked: %q", text)
		}
		var deletions ListResult[ArtifactDeletion]
		call(t, session, "list_artifact_deletions", map[string]any{"project": "alpha", "repository": "api"}, &deletions)
		if len(deletions.Items) != 1 || deletions.Items[0].AffectedTags == nil {
			t.Fatalf("deletions = %#v", deletions)
		}
	}
}

func TestToolInputsAreValidated(t *testing.T) {
	session := connect(t, newTestDeps(), &admin)
	longCursor := strings.Repeat("a", constants.MCPMaxCursorLength+1)
	for name, test := range map[string]struct {
		tool      string
		arguments map[string]any
	}{
		"invalid digest":       {"get_artifact", map[string]any{"project": "alpha", "repository": "api", "digest": "sha256:xyz"}},
		"limit above maximum":  {"list_projects", map[string]any{"limit": constants.MCPMaxPageLimit + 1}},
		"negative limit":       {"list_projects", map[string]any{"limit": -1}},
		"overlong cursor":      {"list_projects", map[string]any{"cursor": longCursor}},
		"garbage cursor":       {"list_projects", map[string]any{"cursor": "not-a-cursor"}},
		"unknown property":     {"get_project", map[string]any{"project": "alpha", "extra": true}},
		"missing project":      {"get_project", map[string]any{}},
		"invalid slug":         {"get_project", map[string]any{"project": "Alpha/../beta"}},
		"traversal repository": {"get_repository", map[string]any{"project": "alpha", "repository": "../beta/secret"}},
		"invalid sort":         {"list_tags", map[string]any{"project": "alpha", "repository": "api", "sort": "random"}},
		"invalid run id":       {"get_lifecycle_run", map[string]any{"project": "alpha", "runId": "nope"}},
		"overlong query":       {"list_artifacts", map[string]any{"project": "alpha", "repository": "api", "q": strings.Repeat("x", constants.MCPMaxQueryRunes+1)}},
	} {
		t.Run(name, func(t *testing.T) {
			callError(t, session, test.tool, test.arguments)
		})
	}
}

func TestToolsFailClosedWithoutPrincipal(t *testing.T) {
	session := connect(t, newTestDeps(), nil)
	if text := callError(t, session, "list_projects", nil); !strings.Contains(text, errUnauthenticated.Error()) {
		t.Fatalf("missing principal error = %q", text)
	}
}

func TestUnavailableReadersReportUnavailable(t *testing.T) {
	deps := newTestDeps()
	deps.Inventory, deps.Lifecycle, deps.Deletions = nil, nil, nil
	session := connect(t, deps, &admin)
	var projects ListResult[Project]
	call(t, session, "list_projects", nil, &projects)
	if projects.Items[0].AccountedUsage.Status != "unavailable" {
		t.Fatalf("usage = %#v", projects.Items[0].AccountedUsage)
	}
	for name, arguments := range map[string]map[string]any{
		"get_artifact":            {"project": "alpha", "repository": "api", "digest": subjectDigest},
		"list_lifecycle_runs":     {"project": "alpha", "repository": "api"},
		"get_lifecycle_run":       {"project": "alpha", "runId": alphaRunID},
		"list_artifact_deletions": {"project": "alpha", "repository": "api"},
	} {
		if text := callError(t, session, name, arguments); !strings.Contains(text, errUnavailable.Error()) {
			t.Errorf("%s = %q", name, text)
		}
	}
}
