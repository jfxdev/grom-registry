package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitystore "github.com/jfxdev/grom/backend/internal/identity/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/mcp"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectstore "github.com/jfxdev/grom/backend/internal/projects/infrastructure/persistence/bun"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	registrystore "github.com/jfxdev/grom/backend/internal/registry/infrastructure/persistence/bun"
)

const (
	mcpSubjectDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mcpSBOMDigest    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type mcpTestStack struct {
	server      *Server
	audit       *serverTestAuditStore
	maintenance *maintenance.Controller
	identity    *identityapp.Service
	readerKey   string
	accountID   foundation.ID
	keyID       foundation.ID
	adminRef    foundation.PrincipalRef
	projects    *projectapp.Service
	registry    *registryapp.RepositoryService
	store       *registrystore.Store
}

func newMCPTestStack(t *testing.T, failureLimit int) *mcpTestStack {
	t.Helper()
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:httpapi-mcp-"+foundation.NewID().String()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	identity := identityapp.New(identitystore.New(db), time.Hour)
	if err := identity.BootstrapAdmin(ctx, "admin@example.com", "admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := identity.Login(ctx, "admin@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	adminRef := principalForUser(admin)
	projects := projectapp.New(projectstore.New(db))
	for _, slug := range []string{"alpha", "beta"} {
		if _, err := projects.Create(ctx, adminRef, true, slug, slug, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	account, err := identity.CreateServiceAccount(ctx, "Agent", "agent", "")
	if err != nil {
		t.Fatal(err)
	}
	key, err := identity.CreateServiceAccountAPIToken(ctx, account.ID, "mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	accountRef := foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: account.ID}
	if err := projects.SetMembership(ctx, adminRef, true, "alpha", accountRef, constants.RoleReader); err != nil {
		t.Fatal(err)
	}
	store := registrystore.New(db)
	repositories := registryapp.NewRepositoryService(store)
	inventory := registryapp.NewInventoryService(store)
	alpha, err := projects.Find(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repositories.Create(ctx, alpha.ID, "api", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, observation := range []registrydomain.ManifestObservation{
		{Digest: mcpSubjectDigest, ManifestSize: 10, Tag: "v1", ArtifactRelationship: constants.ArtifactRelationshipPrimary},
		{Digest: mcpSBOMDigest, ManifestSize: 5, SubjectDigest: mcpSubjectDigest, ArtifactType: "application/spdx+json", ArtifactRelationship: constants.ArtifactRelationshipReferrer},
	} {
		if err := store.UpsertManifestObservation(ctx, repository.ID, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	auditStore := &serverTestAuditStore{}
	audit := auditapp.New(auditStore)
	controller := maintenance.New()
	server, err := New(
		identity, audit, projects, repositories, inventory,
		registryapp.NewArtifactDeletionService(store, repositories, inventory, nil, audit),
		registryapp.NewLifecycleService(store, inventory, nil, audit),
		nil, nil, http.NotFoundHandler(), slog.New(slog.NewTextHandler(io.Discard, nil)),
		"https://grom.example", true, false, "strict", false,
		SecurityOptions{AuthFailureLimit: failureLimit, AuthFailureWindow: time.Minute, AuthBlockDuration: time.Minute},
		OperationalOptions{Database: "sqlite", Maintenance: controller, Version: "test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return &mcpTestStack{
		server: server, audit: auditStore, maintenance: controller, identity: identity,
		readerKey: key.Secret, accountID: account.ID, keyID: key.Token.ID,
		adminRef: adminRef, projects: projects, registry: repositories, store: store,
	}
}

func mcpRequest(body string, headers map[string]string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "https://grom.example/mcp", strings.NewReader(body))
	request.RemoteAddr = "192.0.2.10:4000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request
}

const mcpInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

func TestMCPRejectsMissingAndInvalidBearerCredentials(t *testing.T) {
	stack := newMCPTestStack(t, 10)
	for name, headers := range map[string]map[string]string{
		"missing":      nil,
		"basic":        {"Authorization": "Basic YWdlbnQ6c2VjcmV0"},
		"invalid":      {"Authorization": "Bearer grm_unknown_secret"},
		"session only": {"Cookie": constants.SessionCookieName + "=grms_x_y", "Origin": "https://grom.example"},
		"empty bearer": {"Authorization": "Bearer "},
		"overlong":     {"Authorization": "Bearer " + strings.Repeat("a", constants.MCPMaxBearerLength)},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			stack.server.Handler().ServeHTTP(response, mcpRequest(mcpInitialize, headers))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("WWW-Authenticate"); got != `Bearer realm="Grom MCP"` {
				t.Fatalf("WWW-Authenticate = %q", got)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("expected no-store on MCP responses")
			}
		})
	}
	stack.audit.mu.Lock()
	defer stack.audit.mu.Unlock()
	if len(stack.audit.events) == 0 {
		t.Fatal("expected authentication failures to be audited")
	}
	for _, event := range stack.audit.events {
		if event.Action != constants.AuditRegistryAuthFailed || !strings.Contains(event.MetadataJSON, `"channel":"mcp"`) {
			t.Fatalf("unexpected audit event: %#v", event)
		}
		if strings.Contains(event.MetadataJSON, "grm_") || strings.Contains(event.MetadataJSON, "secret") {
			t.Fatalf("audit metadata leaked a credential: %s", event.MetadataJSON)
		}
	}
}

func TestMCPSharesTheRegistryAuthenticationLimiter(t *testing.T) {
	stack := newMCPTestStack(t, 2)
	for range 2 {
		response := httptest.NewRecorder()
		stack.server.Handler().ServeHTTP(response, mcpRequest(mcpInitialize, map[string]string{"Authorization": "Bearer grm_bad_secret"}))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", response.Code)
		}
	}
	response := httptest.NewRecorder()
	stack.server.Handler().ServeHTTP(response, mcpRequest(mcpInitialize, map[string]string{"Authorization": "Bearer " + stack.readerKey}))
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("expected the blocked client to receive 429, got %d", response.Code)
	}
	tokenRequest := httptest.NewRequest(http.MethodGet, "https://grom.example/auth/token", nil)
	tokenRequest.RemoteAddr = "192.0.2.10:4000"
	tokenRequest.SetBasicAuth("agent", stack.readerKey)
	response = httptest.NewRecorder()
	stack.server.Handler().ServeHTTP(response, tokenRequest)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected /auth/token to share the block, got %d", response.Code)
	}
}

func TestMCPTransportGuards(t *testing.T) {
	stack := newMCPTestStack(t, 10)
	bearer := map[string]string{"Authorization": "Bearer " + stack.readerKey}

	response := httptest.NewRecorder()
	stack.server.Handler().ServeHTTP(response, mcpRequest(mcpInitialize, map[string]string{"Authorization": bearer["Authorization"], "Origin": "https://evil.example"}))
	if response.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin: expected 403, got %d", response.Code)
	}

	response = httptest.NewRecorder()
	stack.server.Handler().ServeHTTP(response, mcpRequest(mcpInitialize, map[string]string{"Authorization": bearer["Authorization"], "Cookie": constants.SessionCookieName + "=grms_x_y"}))
	if response.Code != http.StatusForbidden {
		t.Fatalf("cookie without origin: expected 403, got %d", response.Code)
	}

	get := httptest.NewRequest(http.MethodGet, "https://grom.example/mcp", nil)
	get.Header.Set("Authorization", bearer["Authorization"])
	get.Header.Set("Accept", "application/json, text/event-stream")
	response = httptest.NewRecorder()
	stack.server.Handler().ServeHTTP(response, get)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("stateless GET: expected 405, got %d", response.Code)
	}

	response = httptest.NewRecorder()
	oversized := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"padding":"` + strings.Repeat("x", constants.MCPMaxRequestBytes) + `"}}`
	stack.server.Handler().ServeHTTP(response, mcpRequest(oversized, bearer))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: expected 413, got %d", response.Code)
	}

	for _, path := range []string{"/mcp/extra", "/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"} {
		response = httptest.NewRecorder()
		stack.server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://grom.example"+path, nil))
		if response.Code != http.StatusNotFound || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: expected JSON 404, got %d %q", path, response.Code, response.Header().Get("Content-Type"))
		}
	}

	leave, err := stack.maintenance.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	stack.server.Handler().ServeHTTP(response, mcpRequest(mcpInitialize, bearer))
	leave()
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("backup quiescence: expected 503, got %d", response.Code)
	}
}

type bearerTransport struct {
	token string
}

func (b bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(request)
}

func connectMCP(t *testing.T, stack *mcpTestStack) *mcpsdk.ClientSession {
	t.Helper()
	httpServer := httptest.NewServer(stack.server.Handler())
	t.Cleanup(httpServer.Close)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "grom-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:             httpServer.URL + constants.MCPPath,
		HTTPClient:           &http.Client{Transport: bearerTransport{token: stack.readerKey}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestMCPEndToEndWithAServiceAccountKey(t *testing.T) {
	stack := newMCPTestStack(t, 10)
	session := connectMCP(t, stack)
	if info := session.InitializeResult().ServerInfo; info.Name != "grom" || info.Version != "test" {
		t.Fatalf("server info = %#v", info)
	}

	decode := func(name string, arguments map[string]any, output any) {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s: err=%v result=%#v", name, err, result)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(raw, output); err != nil {
			t.Fatal(err)
		}
	}
	var projects mcp.ListResult[mcp.Project]
	decode("list_projects", nil, &projects)
	if len(projects.Items) != 1 || projects.Items[0].Slug != "alpha" {
		t.Fatalf("projects = %#v", projects)
	}
	var repositories mcp.ListResult[mcp.Repository]
	decode("list_repositories", map[string]any{"project": "alpha"}, &repositories)
	if len(repositories.Items) != 1 || repositories.Items[0].Name != "api" {
		t.Fatalf("repositories = %#v", repositories)
	}
	var artifact mcp.Artifact
	decode("get_artifact", map[string]any{"project": "alpha", "repository": "api", "digest": mcpSubjectDigest}, &artifact)
	if artifact.Digest != mcpSubjectDigest || len(artifact.Tags) != 1 || artifact.Tags[0] != "v1" {
		t.Fatalf("artifact = %#v", artifact)
	}
	var referrers mcp.ReferrersResult
	decode("list_referrers", map[string]any{"project": "alpha", "repository": "api", "digest": mcpSubjectDigest}, &referrers)
	if len(referrers.Items) != 1 || referrers.Items[0].Digest != mcpSBOMDigest {
		t.Fatalf("referrers = %#v", referrers)
	}
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "get_project", Arguments: map[string]any{"project": "beta"}})
	if err != nil || !result.IsError {
		t.Fatalf("expected a non-member project to be hidden, got err=%v result=%#v", err, result)
	}

	if err := stack.identity.RevokeServiceAccountAPIToken(context.Background(), stack.accountID, stack.keyID); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "list_projects"}); err == nil {
		t.Fatal("expected a revoked key to be rejected on the next request")
	}
}

func TestMCPContinuationCursorsFitForLongestAcceptedArguments(t *testing.T) {
	stack := newMCPTestStack(t, 10)
	ctx := context.Background()
	slug := strings.Repeat("p", 40)
	project, err := stack.projects.Create(ctx, stack.adminRef, true, "Long", slug, "", "")
	if err != nil {
		t.Fatal(err)
	}
	accountRef := foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: stack.accountID}
	if err := stack.projects.SetMembership(ctx, stack.adminRef, true, slug, accountRef, constants.RoleReader); err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("r", 200)
	repository, err := stack.registry.Create(ctx, project.ID, name, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// OCI tag names are at most 128 characters, so the longest query that two
	// distinct tags share is 127 characters.
	query := strings.Repeat("t", 127)
	now := time.Now().UTC()
	for i, digest := range []string{"sha256:" + strings.Repeat("c", 64), "sha256:" + strings.Repeat("d", 64)} {
		observation := registrydomain.ManifestObservation{Digest: digest, ManifestSize: 10, Tag: query + string(rune('a'+i))}
		if err := stack.store.UpsertManifestObservation(ctx, repository.ID, observation, now); err != nil {
			t.Fatal(err)
		}
	}

	session := connectMCP(t, stack)
	seen := map[string]bool{}
	arguments := map[string]any{"project": slug, "repository": name, "q": query, "limit": 1}
	for page := 0; page < 2; page++ {
		result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_artifacts", Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("page %d: err=%v result=%#v", page, err, result)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var artifacts mcp.ListResult[mcp.Artifact]
		if err := json.Unmarshal(raw, &artifacts); err != nil {
			t.Fatal(err)
		}
		if len(artifacts.Items) != 1 || seen[artifacts.Items[0].Digest] {
			t.Fatalf("page %d = %#v", page, artifacts)
		}
		seen[artifacts.Items[0].Digest] = true
		if page == 0 {
			if artifacts.NextCursor == "" || len(artifacts.NextCursor) > constants.MCPMaxCursorLength {
				t.Fatalf("first page cursor has length %d", len(artifacts.NextCursor))
			}
			arguments["cursor"] = artifacts.NextCursor
		}
	}
}
