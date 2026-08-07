package registrye2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

const commandTimeout = 2 * time.Minute

func TestRestartPreservesPublicState(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}

	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	admin.createProject(t, "Restart Alpha", "restart-alpha")
	writer := admin.createServicePrincipal(t, "Restart writer", "restart-writer")
	admin.setMembership(t, "restart-alpha", writer, openapi.Writer)

	localTags := make([]string, 0, 8)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })
	variantA, variantB := buildFixtureImages(t, stack, &localTags)
	writerDocker := newDockerClient(t, stack, writer, &localTags)
	writerDocker.login(t, writer.username)
	v1 := writerDocker.tag(t, variantA, "restart-alpha/app", "v1")
	writerDocker.push(t, v1)
	waitForRestartObservation(t, admin, "restart-alpha", "restart-alpha/app", []string{"v1"})

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	output, err := stack.compose(ctx, "restart", "grom", "distribution")
	cancel()
	if err != nil {
		t.Fatalf("restart isolated Grom and Distribution services: %v\n%s", err, bounded(output))
	}
	waitForPublicStack(t, stack.publicURL)

	postRestartAdmin := newManagementClient(t, stack.publicURL)
	postRestartAdmin.login(t)
	waitForRestartObservation(t, postRestartAdmin, "restart-alpha", "restart-alpha/app", []string{"v1"})

	postRestartWriter := newDockerClient(t, stack, writer, &localTags)
	postRestartWriter.login(t, writer.username)
	removeLocalTag(t, stack.root, v1)
	postRestartWriter.pull(t, v1)

	v2 := postRestartWriter.tag(t, variantB, "restart-alpha/app", "v2")
	postRestartWriter.push(t, v2)
	waitForRestartObservation(t, postRestartAdmin, "restart-alpha", "restart-alpha/app", []string{"v1", "v2"})
}

func TestUserDisableRevokesActiveSession(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}

	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)

	const (
		targetEmail    = "disabled-user@grom.local"
		targetUsername = "disabled-user"
		targetPassword = "disabled-user-password"
	)
	target := admin.createUser(t, targetEmail, targetUsername, targetPassword)
	if target.SystemAdmin {
		t.Fatal("target user must not be an installation administrator")
	}

	targetSession := newManagementClient(t, stack.publicURL)
	loggedIn := targetSession.loginAs(t, targetEmail, targetPassword, http.StatusOK)
	if loggedIn.Id != target.Id {
		t.Fatalf("target login returned user %s, expected %s", loggedIn.Id, target.Id)
	}
	active := targetSession.currentUser(t, http.StatusOK)
	if active.Id != target.Id {
		t.Fatalf("active session returned user %s, expected %s", active.Id, target.Id)
	}

	admin.disableUser(t, target.Id.String())
	targetSession.currentUser(t, http.StatusUnauthorized)

	newSession := newManagementClient(t, stack.publicURL)
	newSession.loginAs(t, targetEmail, targetPassword, http.StatusUnauthorized)
}

func TestRegistryJourney(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}

	stack := startTestStack(t)
	api := newManagementClient(t, stack.publicURL)
	api.login(t)

	api.createProject(t, "Alpha", "alpha")
	api.createProject(t, "Beta", "beta")
	writer := api.createServicePrincipal(t, "Alpha writer", "alpha-writer")
	reader := api.createServicePrincipal(t, "Alpha reader", "alpha-reader")
	betaWriter := api.createServicePrincipal(t, "Beta writer", "beta-writer")
	api.setMembership(t, "alpha", writer, openapi.Writer)
	api.setMembership(t, "alpha", reader, openapi.Reader)
	api.setMembership(t, "beta", betaWriter, openapi.Writer)

	localTags := make([]string, 0, 24)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })
	variantA, variantB := buildFixtureImages(t, stack, &localTags)
	writerDocker := newDockerClient(t, stack, writer, &localTags)
	readerDocker := newDockerClient(t, stack, reader, &localTags)
	betaDocker := newDockerClient(t, stack, betaWriter, &localTags)

	appTarget := writerDocker.tag(t, variantA, "alpha/app", "v1")
	if !t.Run("phase 1: first push and normal access", func(t *testing.T) {
		writerDocker.login(t, writer.username)
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		defer cancel()
		token, _, err := api.exchangeToken(ctx, writer.username, writer.secret, "alpha/app")
		if err != nil {
			t.Fatalf("exchange first-push Writer token: %v", err)
		}
		assertTokenAction(t, token.Token, "alpha/app", "push", true)
		writerDocker.push(t, appTarget)

		removeLocalTag(t, stack.root, appTarget)
		writerDocker.pull(t, appTarget)
		removeLocalTag(t, stack.root, appTarget)
		readerDocker.login(t, reader.username)
		readerDocker.pull(t, appTarget)
	}) {
		return
	}

	if !t.Run("phase 2: role and project denial", func(t *testing.T) {
		readerDenied := readerDocker.tag(t, variantA, "alpha/app", "reader-denied")
		assertRegistryDenied(t, readerDocker.pushDenied(t, readerDenied))

		betaDocker.login(t, betaWriter.username)
		removeLocalTag(t, stack.root, appTarget)
		existingOutput := betaDocker.pullDenied(t, appTarget)
		missingTarget := fmt.Sprintf("%s/alpha/missing:v1", stack.registry)
		missingOutput := betaDocker.pullDenied(t, missingTarget)
		existingClass := registryDenialClass(existingOutput)
		missingClass := registryDenialClass(missingOutput)
		if existingClass == "" || missingClass == "" || existingClass != missingClass {
			t.Fatalf("existing and missing private repositories must have equivalent denial posture; existing=%q missing=%q",
				existingClass, missingClass)
		}

		crossProjectPush := betaDocker.tag(t, variantA, "alpha/forbidden", "v1")
		assertRegistryDenied(t, betaDocker.pushDenied(t, crossProjectPush))
	}) {
		return
	}

	if !t.Run("phase 3: JWT lifetime and mutable authorization", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		defer cancel()
		token, _, err := api.exchangeToken(ctx, writer.username, writer.secret, "alpha/app")
		if err != nil {
			t.Fatalf("exchange Writer access key for scoped JWT: %v", err)
		}
		assertTokenAction(t, token.Token, "alpha/app", "pull", true)
		if status, err := api.manifest(ctx, token.Token, "alpha/app", "v1"); err != nil || status != http.StatusOK {
			t.Fatalf("read manifest with fresh JWT: status=%d error=%v", status, err)
		}

		expiry := tokenExpiry(t, token.Token)
		waitForJWTRejection(t, ctx, api, token.Token, expiry, "alpha/app", "v1")

		fresh, _, err := api.exchangeToken(ctx, writer.username, writer.secret, "alpha/app")
		if err != nil {
			t.Fatalf("exchange still-valid access key after JWT expiry: %v", err)
		}
		if status, err := api.manifest(ctx, fresh.Token, "alpha/app", "v1"); err != nil || status != http.StatusOK {
			t.Fatalf("read manifest with refreshed JWT: status=%d error=%v", status, err)
		}

		api.revokeAccessKey(t, writer)
		if _, status, err := api.exchangeToken(ctx, writer.username, writer.secret, "alpha/app"); err == nil ||
			status != http.StatusUnauthorized {
			t.Fatalf("revoked access key exchange: status=%d error=%v", status, err)
		}

		readerToken, _, err := api.exchangeToken(ctx, reader.username, reader.secret, "alpha/app")
		if err != nil {
			t.Fatalf("exchange Reader access key before membership removal: %v", err)
		}
		assertTokenAction(t, readerToken.Token, "alpha/app", "pull", true)
		api.deleteMembership(t, "alpha", reader)
		removedToken, _, err := api.exchangeToken(ctx, reader.username, reader.secret, "alpha/app")
		if err != nil {
			t.Fatalf("valid Reader key exchange after membership removal: %v", err)
		}
		assertTokenAction(t, removedToken.Token, "alpha/app", "pull", false)
		if status, err := api.manifest(ctx, removedToken.Token, "alpha/app", "v1"); err != nil {
			t.Fatalf("manifest denial after Reader membership removal: %v", err)
		} else if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Fatalf("removed Reader membership returned HTTP %d, expected registry denial", status)
		}

		// The revoked key remains revoked. A distinct reveal-once key lets the
		// same Writer principal exercise the remaining policy scenarios.
		writer.tokenID, writer.secret = api.createAccessKey(t, writer.id, "registry-e2e-policy")
		writerDocker = newDockerClient(t, stack, writer, &localTags)
	}) {
		return
	}

	if !t.Run("phase 4: policy enforcement through Docker", func(t *testing.T) {
		writerDocker.login(t, writer.username)
		trueValue := true
		protectedPatterns := []string{"prod"}
		api.createRepository(t, "alpha", "protected", []openapi.RepositoryPolicyInput{{
			Type: openapi.TagProtection, Enabled: true, TagPatterns: &protectedPatterns,
			PreventOverwrite: &trueValue,
		}})
		protectedTarget := writerDocker.tag(t, variantA, "alpha/protected", "prod")
		writerDocker.push(t, protectedTarget)
		writerDocker.tag(t, variantB, "alpha/protected", "prod")
		assertManifestPutDenied(t, writerDocker.pushDenied(t, protectedTarget))

		immutablePatterns := []string{"release-*"}
		api.createRepository(t, "alpha", "immutable", []openapi.RepositoryPolicyInput{{
			Type: openapi.Immutability, Enabled: true, TagPatterns: &immutablePatterns,
			PreventOverwrite: &trueValue,
		}})
		immutableTarget := writerDocker.tag(t, variantA, "alpha/immutable", "release-1")
		writerDocker.push(t, immutableTarget)
		writerDocker.tag(t, variantB, "alpha/immutable", "release-1")
		assertManifestPutDenied(t, writerDocker.pushDenied(t, immutableTarget))

		allowedPatterns := []string{"v*"}
		api.createRepository(t, "alpha", "named", []openapi.RepositoryPolicyInput{{
			Type: openapi.TagNaming, Enabled: true, AllowedPatterns: &allowedPatterns,
		}})
		namedAllowed := writerDocker.tag(t, variantA, "alpha/named", "v1")
		writerDocker.push(t, namedAllowed)
		namedDenied := writerDocker.tag(t, variantA, "alpha/named", "latest")
		assertManifestPutDenied(t, writerDocker.pushDenied(t, namedDenied))
	}) {
		return
	}

	t.Run("phase 5: repository observation", func(t *testing.T) {
		deadline := time.Now().Add(15 * time.Second)
		for {
			repositories := api.repositories(t, "alpha")
			tags := api.tags(t, "alpha", "app")
			inventory := api.inventory(t, "alpha", "app")
			if observationComplete(repositories, tags, inventory) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("push observation did not converge: repositories=%#v tags=%#v inventory=%#v",
					repositories, tags, inventory)
			}
			time.Sleep(250 * time.Millisecond)
		}
	})
}

type registryJWTClaims struct {
	Access []struct {
		Type    string   `json:"type"`
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	} `json:"access"`
	jwt.RegisteredClaims
}

func parseRegistryClaims(t *testing.T, raw string) *registryJWTClaims {
	t.Helper()
	claims := new(registryJWTClaims)
	if _, _, err := new(jwt.Parser).ParseUnverified(raw, claims); err != nil {
		t.Fatalf("parse registry JWT claims: %v", err)
	}
	return claims
}

func tokenExpiry(t *testing.T, raw string) time.Time {
	t.Helper()
	claims := parseRegistryClaims(t, raw)
	if claims.ExpiresAt == nil {
		t.Fatal("registry JWT has no expiry")
	}
	return claims.ExpiresAt.Time
}

func waitForJWTRejection(
	t *testing.T,
	ctx context.Context,
	api *managementClient,
	raw string,
	expiry time.Time,
	repository, reference string,
) {
	t.Helper()
	deadline := expiry.Add(75 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("Distribution still accepted a JWT 75 seconds after its signed expiry")
		}
		status, err := api.manifest(ctx, raw, repository, reference)
		if err != nil {
			t.Fatalf("reuse registry JWT while waiting for expiry rejection: %v", err)
		}
		if status == http.StatusUnauthorized {
			return
		}
		if status != http.StatusOK {
			t.Fatalf("registry JWT expiry check returned unexpected HTTP %d", status)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("waiting for registry JWT rejection: %v", ctx.Err())
		case <-timer.C:
		}
	}
}

func assertTokenAction(t *testing.T, raw, repository, action string, expected bool) {
	t.Helper()
	found := false
	for _, access := range parseRegistryClaims(t, raw).Access {
		if access.Type != "repository" || access.Name != repository {
			continue
		}
		for _, candidate := range access.Actions {
			if candidate == action {
				found = true
			}
		}
	}
	if found != expected {
		t.Fatalf("registry JWT action %q for %q: got %t, expected %t", action, repository, found, expected)
	}
}

func assertRegistryDenied(t *testing.T, output string) {
	t.Helper()
	if registryDenialClass(output) == "" {
		t.Fatalf("Docker failed without a recognizable registry denial:\n%s", bounded(output))
	}
}

func registryDenialClass(output string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "denied"), strings.Contains(lower, "requested access"):
		return "denied"
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "authentication required"):
		return "unauthorized"
	case strings.Contains(lower, "forbidden"):
		return "forbidden"
	default:
		return ""
	}
}

func assertManifestPutDenied(t *testing.T, output string) {
	t.Helper()
	lower := strings.ToLower(output)
	policyMessage := strings.Contains(lower, "immutable for this repository") ||
		strings.Contains(lower, "naming policy")
	if !strings.Contains(lower, "denied") || !policyMessage {
		t.Fatalf("Docker push did not expose the gateway's policy denial:\n%s", bounded(output))
	}
}

func observationComplete(
	repositories []openapi.Repository,
	tags openapi.TagList,
	inventory []openapi.ManifestInventory,
) bool {
	repositoryOK := false
	for _, repository := range repositories {
		if repository.Name == "app" &&
			repository.CreationSource == openapi.Push &&
			repository.Status == openapi.RepositoryStatusActive &&
			repository.Profile == openapi.RepositoryProfileContainerImage {
			repositoryOK = true
		}
	}
	tagOK := tags.Name == "alpha/app" || tags.Name == "app"
	tagOK = tagOK && contains(tags.Tags, "v1")
	inventoryOK := false
	for _, manifest := range inventory {
		if manifest.Digest != "" &&
			manifest.ObservedKind == openapi.ArtifactKindContainerImage &&
			manifest.ArtifactRelationship == openapi.Primary &&
			contains(manifest.Tags, "v1") {
			inventoryOK = true
		}
	}
	return repositoryOK && tagOK && inventoryOK
}

func waitForRestartObservation(t *testing.T, api *managementClient, projectSlug, repository string, expectedTags []string) {
	t.Helper()
	repositoryName := strings.TrimPrefix(repository, projectSlug+"/")
	deadline := time.Now().Add(15 * time.Second)
	for {
		repositories := api.repositories(t, projectSlug)
		tags := api.tags(t, projectSlug, repositoryName)
		inventory := api.inventory(t, projectSlug, repositoryName)
		if restartObservationComplete(repositories, tags, inventory, repository, repositoryName, expectedTags) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("restart observation did not converge: repositories=%#v tags=%#v inventory=%#v",
				repositories, tags, inventory)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func restartObservationComplete(
	repositories []openapi.Repository,
	tags openapi.TagList,
	inventory []openapi.ManifestInventory,
	repository, repositoryName string,
	expectedTags []string,
) bool {
	repositoryOK := false
	for _, repository := range repositories {
		if repository.Name == repositoryName && repository.Status == openapi.RepositoryStatusActive {
			repositoryOK = true
			break
		}
	}
	if !repositoryOK || (tags.Name != repository && tags.Name != repositoryName) {
		return false
	}
	for _, expectedTag := range expectedTags {
		if !contains(tags.Tags, expectedTag) {
			return false
		}
		inventoryTagObserved := false
		for _, manifest := range inventory {
			if manifest.Digest != "" && contains(manifest.Tags, expectedTag) {
				inventoryTagObserved = true
				break
			}
		}
		if !inventoryTagObserved {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
