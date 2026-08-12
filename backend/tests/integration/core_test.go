package integration

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	identitystore "github.com/jfxdev/grom/backend/internal/identity/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectstore "github.com/jfxdev/grom/backend/internal/projects/infrastructure/persistence/bun"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	registrystore "github.com/jfxdev/grom/backend/internal/registry/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
	"github.com/uptrace/bun"
)

func TestSQLiteCoreFlow(t *testing.T) {
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:grom-core-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}

	identityService := identityapp.New(identitystore.New(db), time.Hour)
	if err := identityService.BootstrapAdmin(ctx, "admin@example.com", "admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := identityService.Login(ctx, "admin@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	runRepositoryPersistenceFlow(t, ctx, db, identityService, admin)
}

func TestSQLiteRestartPreservesCoreState(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "grom.db")
	databaseURL := "sqlite://" + databasePath

	db, kind, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	identityService := identityapp.New(identitystore.New(db), time.Hour)
	if err := identityService.BootstrapAdmin(ctx, "restart@example.com", "restart-admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := identityService.Login(ctx, "restart@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	projectService := projectapp.New(projectstore.New(db))
	project, err := projectService.Create(ctx, foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID}, true, "Restart", "restart")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, kind, err = database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	identityService = identityapp.New(identitystore.New(db), time.Hour)
	if _, _, err := identityService.Login(ctx, "restart@example.com", "secret-password"); err != nil {
		t.Fatalf("admin did not survive restart: %v", err)
	}
	projectService = projectapp.New(projectstore.New(db))
	restored, err := projectService.Find(ctx, project.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != project.ID || restored.Slug != project.Slug {
		t.Fatalf("project changed across restart: before=%+v after=%+v", project, restored)
	}
}

func runRepositoryPersistenceFlow(
	t *testing.T,
	ctx context.Context,
	db *bun.DB,
	identityService *identityapp.Service,
	admin *identitydomain.User,
) {
	t.Helper()

	projectService := projectapp.New(projectstore.New(db))
	registryStore := registrystore.New(db)
	repositoryService := registryapp.NewRepositoryService(registryStore)
	projectService.SetDeletionGuard(repositoryService)

	createdDeveloper, err := identityService.CreateUser(ctx, "developer@example.com", "developer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identityService.CompletePasswordReset(ctx, createdDeveloper.RegistrationLink.Token, "developer-password"); err != nil {
		t.Fatal(err)
	}
	developer := &createdDeveloper.User
	developerSession, _, err := identityService.Login(ctx, "developer@example.com", "developer-password")
	if err != nil {
		t.Fatal(err)
	}
	firstReset, err := identityService.CreatePasswordReset(ctx, developer.ID)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := identityService.CreatePasswordReset(ctx, developer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := identityService.Login(ctx, "developer@example.com", "developer-password"); err != nil {
		t.Fatal("creating a reset link must not change the password before the link is used")
	}
	if _, err := identityService.CompletePasswordReset(ctx, firstReset.Token, "replacement-password"); err == nil {
		t.Fatal("expected a newer reset link to invalidate the previous link")
	}
	if resetUserID, err := identityService.CompletePasswordReset(ctx, reset.Token, "replacement-password"); err != nil {
		t.Fatal(err)
	} else if resetUserID != developer.ID {
		t.Fatalf("unexpected reset user: %s", resetUserID)
	}
	if _, _, err := identityService.Login(ctx, "developer@example.com", "developer-password"); err == nil {
		t.Fatal("expected old password to stop working after reset completion")
	}
	if _, _, err := identityService.Login(ctx, "developer@example.com", "replacement-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := identityService.AuthenticateSession(ctx, developerSession); err == nil {
		t.Fatal("expected password reset completion to revoke existing sessions")
	}
	if _, err := identityService.CompletePasswordReset(ctx, reset.Token, "another-password"); err == nil {
		t.Fatal("expected password reset link to be single use")
	}

	if _, err := projectService.Create(ctx, foundation.PrincipalRef{
		Kind: constants.PrincipalUser, ID: developer.ID,
	}, false, "Unauthorized", "unauthorized"); err == nil {
		t.Fatal("expected project creation to require an installation administrator")
	}
	project, err := projectService.Create(ctx, foundation.PrincipalRef{
		Kind: constants.PrincipalUser, ID: admin.ID,
	}, true, "Payments", "payments")
	if err != nil {
		t.Fatal(err)
	}
	viewerCreated, err := identityService.CreateUser(ctx, "viewer@example.com", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identityService.CompletePasswordReset(ctx, viewerCreated.RegistrationLink.Token, "viewer-password"); err != nil {
		t.Fatal(err)
	}
	viewer, err := identityService.PromoteUserToSystemViewer(ctx, viewerCreated.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	viewerCreatedToken, err := identityService.CreateViewerRegistryToken(ctx, viewer.ID, "local pull", nil)
	if err != nil {
		t.Fatal(err)
	}
	viewerPrincipal, err := identityService.AuthenticateRegistry(ctx, viewer.Username, viewerCreatedToken.Secret)
	if err != nil || viewerPrincipal.Kind != constants.PrincipalUser || viewerPrincipal.ID != viewer.ID {
		t.Fatalf("authenticate viewer registry token: %#v, %v", viewerPrincipal, err)
	}
	viewerSigner, err := signing.LoadOrCreate(filepath.Join(t.TempDir(), "viewer.key"), filepath.Join(t.TempDir(), "viewer.crt"))
	if err != nil {
		t.Fatal(err)
	}
	viewerTokenService := registryapp.NewTokenService(projectService, nil, viewerSigner, time.Minute)
	withoutMembership, _, _, err := viewerTokenService.Issue(ctx, viewer.Username, viewerPrincipal, constants.RegistryService, []string{"repository:payments/api:pull"})
	if err != nil || viewerTokenService.HasAccess(withoutMembership, "payments/api", constants.RegistryActionPull) {
		t.Fatalf("viewer must not receive registry access without membership: %v", err)
	}
	if err := projectService.SetMembership(ctx, foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID}, true, project.Slug, viewerPrincipal, constants.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	viewerJWT, _, _, err := viewerTokenService.Issue(ctx, viewer.Username, viewerPrincipal, constants.RegistryService, []string{"repository:payments/api:pull,push,delete"})
	if err != nil || !viewerTokenService.HasAccess(viewerJWT, "payments/api", constants.RegistryActionPull) || viewerTokenService.HasAccess(viewerJWT, "payments/api", constants.RegistryActionPush) || viewerTokenService.HasAccess(viewerJWT, "payments/api", constants.RegistryActionDelete) {
		t.Fatalf("viewer registry token must be pull-only: %v", err)
	}
	account, err := identityService.CreateServiceAccount(ctx, "CI", "ci-payments", "")
	if err != nil {
		t.Fatal(err)
	}
	principal := foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: account.ID}
	if err := projectService.SetMembership(ctx,
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID},
		true, project.Slug, principal, constants.RoleWriter,
	); err != nil {
		t.Fatal(err)
	}
	created, err := identityService.CreateServiceAccountAPIToken(ctx, account.ID, "pipeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := identityService.ListServiceAccountAPITokens(ctx, account.ID)
	if err != nil || len(tokens) != 1 || tokens[0].ServiceAccountID != account.ID {
		t.Fatalf("list service account tokens: %#v, %v", tokens, err)
	}
	authenticated, err := identityService.AuthenticateRegistry(ctx, account.Username, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	allowed := projectService.AllowedActions(ctx, authenticated, "payments/api",
		[]string{constants.RegistryActionPull, constants.RegistryActionPush, constants.RegistryActionDelete})
	if len(allowed) != 2 || allowed[0] != constants.RegistryActionPull || allowed[1] != constants.RegistryActionPush {
		t.Fatalf("unexpected actions: %#v", allowed)
	}
	if denied := projectService.AllowedActions(ctx, authenticated, "other/api", []string{constants.RegistryActionPull}); len(denied) != 0 {
		t.Fatalf("expected cross-project access to be denied: %#v", denied)
	}
	repository, err := repositoryService.Create(ctx, project.ID, "api", "Payments API", []registrydomain.Policy{{
		Type: constants.RepositoryPolicyTagProtection, Enabled: true, TagPatterns: []string{"prod"},
		PreventOverwrite: true, PreventDeletion: true, ExcludeFromLifecycle: true,
	}, {
		Type: constants.RepositoryPolicyManualDeletion, Enabled: true, RequireReason: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if repository.Status != constants.RepositoryStatusEmpty ||
		repository.Profile != constants.RepositoryProfileUnknown ||
		len(repository.Policies) != 2 {
		t.Fatalf("unexpected repository: %#v", repository)
	}
	observedAt := time.Now().UTC().Add(-time.Hour)
	if err := registryStore.UpsertManifestObservation(ctx, repository.ID, registrydomain.ManifestObservation{
		Digest: "sha256:inventory", MediaType: "application/vnd.oci.image.manifest.v1+json",
		Tag: "dev", PushedAt: &observedAt,
	}, observedAt); err != nil {
		t.Fatal(err)
	}
	inventory, err := registryStore.ListManifestInventory(ctx, repository.ID)
	if err != nil || len(inventory) != 1 || len(inventory[0].Tags) != 1 || inventory[0].Tags[0] != "dev" {
		t.Fatalf("unexpected registry inventory: %#v, %v", inventory, err)
	}
	reconciledAt := observedAt.Add(2 * time.Hour)
	if err := registryStore.CompleteInventoryReconciliation(
		ctx, repository.ID, []string{}, []string{}, reconciledAt,
	); err != nil {
		t.Fatal(err)
	}
	inventory, err = registryStore.ListManifestInventory(ctx, repository.ID)
	if err != nil || len(inventory) != 1 || inventory[0].State != constants.InventoryStateMissing ||
		inventory[0].UntaggedAt == nil {
		t.Fatalf("expected an unseen detached manifest to become missing while retaining its history: %#v, %v", inventory, err)
	}
	if err := repositoryService.EvaluateTagMutation(ctx, project.ID, "api", "prod", true); err == nil {
		t.Fatal("expected protected production tag overwrite to be rejected")
	}
	if err := repositoryService.EvaluateTagMutation(ctx, project.ID, "api", "dev", true); err != nil {
		t.Fatalf("expected unprotected tag overwrite to pass: %v", err)
	}
	if _, err := repositoryService.EvaluateDeletion(ctx, project.ID, "api", []string{"prod"}, "cleanup", true); err == nil {
		t.Fatal("expected protected production tag deletion to be rejected")
	}
	if requiresReason, err := repositoryService.EvaluateDeletion(ctx, project.ID, "api", []string{"dev"}, "", true); err == nil || !requiresReason {
		t.Fatalf("expected manual deletion reason to be required: %v", err)
	}
	if _, err := repositoryService.EvaluateDeletion(ctx, project.ID, "api", []string{"dev"}, "cleanup", true); err != nil {
		t.Fatalf("expected reasoned development tag deletion to pass: %v", err)
	}
	storedRepositories, err := repositoryService.List(ctx, project.ID, nil, false)
	if err != nil || len(storedRepositories) != 1 || storedRepositories[0].Name != "api" {
		t.Fatalf("list logical repositories: %#v, %v", storedRepositories, err)
	}
	if repository.PolicyVersion != 1 {
		t.Fatalf("expected initial policy version 1, got %d", repository.PolicyVersion)
	}
	updatedRepository, err := repositoryService.ReplacePolicies(
		ctx, project.ID, repository.ID, repository.PolicyVersion,
		[]registrydomain.Policy{{
			Type: constants.RepositoryPolicyRetention, Enabled: true,
			ExpireAfterDays: intPointer(30), KeepLast: intPointer(5),
		}},
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRepository.PolicyVersion != 2 || len(updatedRepository.Policies) != 1 {
		t.Fatalf("unexpected updated policies: %#v", updatedRepository)
	}
	if _, err := repositoryService.ReplacePolicies(
		ctx, project.ID, repository.ID, 1, nil,
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID},
	); err == nil {
		t.Fatal("expected stale policy version to be rejected")
	}
	previewTime := time.Now().UTC().Add(-31 * time.Minute)
	preview := &registrydomain.LifecyclePreview{
		ID: foundation.NewID(), RepositoryID: repository.ID,
		Status: constants.LifecyclePreviewReady, InventoryAt: previewTime,
		CreatedBy: admin.ID, CreatedAt: previewTime, ExpiresAt: time.Now().UTC().Add(time.Hour),
		PolicyVersion:    updatedRepository.PolicyVersion,
		EvaluatorVersion: constants.LifecycleEvaluatorVersion,
		PolicySnapshot:   "[]",
	}
	if err := registryStore.CreateLifecyclePreview(ctx, preview); err != nil {
		t.Fatal(err)
	}
	interruptedRun := &registrydomain.LifecycleRun{
		ID: foundation.NewID(), PreviewID: preview.ID, RepositoryID: repository.ID,
		ActorID: admin.ID, Reason: "interrupted test",
		Status: constants.LifecycleRunRunning, StartedAt: previewTime,
	}
	if err := registryStore.AcquireLifecycleLock(ctx, repository.ID, interruptedRun.ID, previewTime); err != nil {
		t.Fatal(err)
	}
	if err := registryStore.CreateLifecycleRunFromPreview(ctx, interruptedRun); err != nil {
		t.Fatal(err)
	}
	replacementRunID := foundation.NewID()
	if err := registryStore.AcquireLifecycleLock(
		ctx, repository.ID, replacementRunID, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registryStore.ReleaseLifecycleLock(ctx, repository.ID, replacementRunID); err != nil {
			t.Errorf("release lifecycle lock: %v", err)
		}
	})
	recoveredRun, err := registryStore.FindLifecycleRun(ctx, interruptedRun.ID)
	if err != nil || recoveredRun.Status != constants.LifecycleRunFailed {
		t.Fatalf("expected stale lifecycle run recovery: %#v, %v", recoveredRun, err)
	}
	recoveredPreview, err := registryStore.FindLifecyclePreview(ctx, preview.ID)
	if err != nil || recoveredPreview.Status != constants.LifecyclePreviewExecuted {
		t.Fatalf("expected stale preview to be consumed: %#v, %v", recoveredPreview, err)
	}
	if err := registryStore.CreateLifecycleRunFromPreview(ctx, &registrydomain.LifecycleRun{
		ID: foundation.NewID(), PreviewID: preview.ID, RepositoryID: repository.ID,
		ActorID: admin.ID, Reason: "duplicate", Status: constants.LifecycleRunRunning,
		StartedAt: time.Now().UTC(),
	}); err == nil {
		t.Fatal("expected a preview to create at most one lifecycle run")
	}

	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	tokenService := registryapp.NewTokenService(projectService, repositoryService, signer, time.Minute)
	jwtToken, _, _, err := tokenService.Issue(ctx, account.Username, principal, constants.RegistryService,
		[]string{"repository:payments/api:pull,push"})
	if err != nil || jwtToken == "" {
		t.Fatalf("issue registry JWT: %v", err)
	}
	if !tokenService.HasAccess(jwtToken, "payments/api", constants.RegistryActionPush) {
		t.Fatal("expected registered repository token to contain push access")
	}
	repositoryService.SetExistingRepositoryProbe(func(_ context.Context, fullName string) (bool, error) {
		return fullName == "payments/legacy", nil
	})
	legacyToken, _, _, err := tokenService.Issue(ctx, account.Username, principal, constants.RegistryService,
		[]string{"repository:payments/legacy:pull,push"})
	if err != nil || !tokenService.HasAccess(legacyToken, "payments/legacy", constants.RegistryActionPush) {
		t.Fatalf("expected discovered legacy repository to retain push access: %v", err)
	}
	importedRepositories, err := repositoryService.List(ctx, project.ID, nil, false)
	if err != nil || len(importedRepositories) != 2 {
		t.Fatalf("expected legacy repository import: %#v, %v", importedRepositories, err)
	}
	pullOnlyToken, _, _, err := tokenService.Issue(ctx, account.Username, principal, constants.RegistryService,
		[]string{"repository:payments/unregistered:pull"})
	if err != nil || !tokenService.HasAccess(pullOnlyToken, "payments/unregistered", constants.RegistryActionPull) {
		t.Fatalf("issue pull-only registry JWT: %v", err)
	}
	if exists, err := registryStore.RepositoryExists(ctx, project.ID, "unregistered"); err != nil || exists {
		t.Fatalf("pull-only token unexpectedly persisted a repository: %v, exists=%v", err, exists)
	}
	unregisteredToken, _, _, err := tokenService.Issue(ctx, account.Username, principal, constants.RegistryService,
		[]string{"repository:payments/unregistered:pull,push"})
	if err != nil || unregisteredToken == "" {
		t.Fatalf("issue first-push registry JWT: %v", err)
	}
	var firstPushClaims registryapp.RegistryClaims
	if _, _, err := new(jwt.Parser).ParseUnverified(unregisteredToken, &firstPushClaims); err != nil {
		t.Fatal(err)
	}
	if len(firstPushClaims.Access) != 1 || len(firstPushClaims.Access[0].Actions) != 2 ||
		firstPushClaims.Access[0].Actions[0] != constants.RegistryActionPull ||
		firstPushClaims.Access[0].Actions[1] != constants.RegistryActionPush {
		t.Fatalf("unexpected first-push repository actions: %#v", firstPushClaims.Access)
	}
	if !tokenService.HasAccess(unregisteredToken, "payments/unregistered", constants.RegistryActionPush) {
		t.Fatal("first token for a writer must contain push access")
	}
	pushRepository, err := repositoryService.Find(ctx, project.ID, "unregistered")
	if err != nil || pushRepository.Status != constants.RepositoryStatusEmpty ||
		pushRepository.CreationSource != constants.RepositoryCreationPush ||
		len(pushRepository.Policies) != 0 {
		t.Fatalf("unexpected push-created repository: %#v, %v", pushRepository, err)
	}
	secondPushToken, _, _, err := tokenService.Issue(ctx, account.Username, principal, constants.RegistryService,
		[]string{"repository:payments/unregistered:pull,push"})
	if err != nil || !tokenService.HasAccess(secondPushToken, "payments/unregistered", constants.RegistryActionPush) {
		t.Fatalf("expected push provisioning to be idempotent: %v", err)
	}
	otherAccount, err := identityService.CreateServiceAccount(ctx, "Other CI", "ci-other", "")
	if err != nil {
		t.Fatal(err)
	}
	readerPrincipal := foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: otherAccount.ID}
	if err := projectService.SetMembership(ctx,
		foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: admin.ID},
		true, project.Slug, readerPrincipal, constants.RoleReader,
	); err != nil {
		t.Fatal(err)
	}
	readerToken, _, _, err := tokenService.Issue(ctx, otherAccount.Username, readerPrincipal, constants.RegistryService,
		[]string{"repository:payments/reader-only:pull,push"})
	if err != nil {
		t.Fatal(err)
	}
	if tokenService.HasAccess(readerToken, "payments/reader-only", constants.RegistryActionPush) {
		t.Fatal("reader token must not contain push access")
	}
	if exists, err := registryStore.RepositoryExists(ctx, project.ID, "reader-only"); err != nil || exists {
		t.Fatalf("reader push request unexpectedly persisted a repository: %v, exists=%v", err, exists)
	}
	if err := projectService.Delete(ctx, true, project.Slug); !errors.Is(err, projectapp.ErrProjectNotEmpty) {
		t.Fatalf("expected repository-bearing project deletion to be blocked: %v", err)
	}
	emptyProject, err := projectService.Create(ctx, foundation.PrincipalRef{
		Kind: constants.PrincipalUser, ID: admin.ID,
	}, true, "Disposable", "disposable")
	if err != nil {
		t.Fatal(err)
	}
	if err := projectService.Delete(ctx, false, emptyProject.Slug); err == nil {
		t.Fatal("expected project deletion to require an installation administrator")
	}
	if err := projectService.Delete(ctx, true, emptyProject.Slug); err != nil {
		t.Fatalf("delete empty project: %v", err)
	}
	if _, err := projectService.Find(ctx, emptyProject.Slug); err == nil {
		t.Fatal("expected deleted project to be absent")
	}
	if err := identityService.RevokeServiceAccountAPIToken(ctx, otherAccount.ID, created.Token.ID); err == nil {
		t.Fatal("expected cross-account token revocation to be denied")
	}
	if err := identityService.RevokeServiceAccountAPIToken(ctx, account.ID, created.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := identityService.AuthenticateRegistry(ctx, account.Username, created.Secret); err == nil {
		t.Fatal("expected revoked service account token authentication to fail")
	}

	firstMovedAt := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Minute)
	if err := registryStore.UpsertManifestObservation(ctx, repository.ID, registrydomain.ManifestObservation{
		Digest: "sha256:moved-from", Tag: "moving",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
	}, firstMovedAt); err != nil {
		t.Fatal(err)
	}
	lastPushedAt := firstMovedAt.Add(time.Minute)
	if err := registryStore.UpsertManifestObservation(ctx, repository.ID, registrydomain.ManifestObservation{
		Digest: "sha256:moved-to", Tag: "moving",
		MediaType: "application/vnd.oci.image.manifest.v1+json", PushedAt: &lastPushedAt,
	}, lastPushedAt); err != nil {
		t.Fatal(err)
	}
	if err := registryStore.UpsertManifestObservation(ctx, repository.ID, registrydomain.ManifestObservation{
		Digest: "sha256:moved-to", Tag: "moving",
		MediaType: "application/vnd.oci.image.manifest.v1+json",
	}, lastPushedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	inventory, err = registryStore.ListManifestInventory(ctx, repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	inventoryByDigest := make(map[string]registrydomain.ManifestInventory, len(inventory))
	for _, manifest := range inventory {
		inventoryByDigest[manifest.Digest] = manifest
	}
	movedFrom := inventoryByDigest["sha256:moved-from"]
	if movedFrom.State != constants.InventoryStateUntagged || movedFrom.UntaggedAt == nil {
		t.Fatalf("expected the previous tag target to become untagged: %#v", movedFrom)
	}
	movedTo := inventoryByDigest["sha256:moved-to"]
	if movedTo.State != constants.InventoryStateActive || len(movedTo.Tags) != 1 ||
		movedTo.Tags[0] != "moving" || movedTo.LastPushedAt == nil ||
		!movedTo.LastPushedAt.Equal(lastPushedAt) {
		t.Fatalf("unexpected current tag target after upsert: %#v", movedTo)
	}
	if _, err := db.NewDelete().
		Table("lifecycle_previews").
		Where("id = ?", preview.ID.String()).
		Exec(ctx); err != nil {
		t.Fatalf("delete lifecycle preview with dependent run: %v", err)
	}
	runExists, err := db.NewSelect().
		Table("lifecycle_runs").
		Where("preview_id = ?", preview.ID.String()).
		Exists(ctx)
	if err != nil || runExists {
		t.Fatalf("expected lifecycle runs to cascade with their preview: exists=%v, err=%v", runExists, err)
	}
}

func intPointer(value int) *int {
	return &value
}

func TestPostgresMigrationsAndBootstrap(t *testing.T) {
	databaseURL := os.Getenv("GROM_TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("GROM_TEST_POSTGRES_URL is not configured")
	}
	ctx := context.Background()
	db, kind, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close PostgreSQL database: %v", err)
		}
	})
	if kind != database.Postgres {
		t.Fatalf("expected postgres kind, got %s", kind)
	}
	if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	service := identityapp.New(identitystore.New(db), time.Hour)
	if err := service.BootstrapAdmin(ctx, "postgres-admin@example.com", "postgres-admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	_, admin, err := service.Login(ctx, "postgres-admin@example.com", "secret-password")
	if err != nil {
		t.Fatal(err)
	}
	runRepositoryPersistenceFlow(t, ctx, db, service, admin)
}
