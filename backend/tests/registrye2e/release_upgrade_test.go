package registrye2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

const (
	defaultUpgradeBaseImage = "ghcr.io/jfxdev/grom-registry:0.0.1"
	defaultUpgradePlatform  = "linux/amd64"
)

func TestTaggedReleaseUpgradePreservesRegistryState(t *testing.T) {
	if os.Getenv("GROM_RUN_RELEASE_UPGRADE_E2E") != "1" {
		t.Skip("set GROM_RUN_RELEASE_UPGRADE_E2E=1 or run make test-release-upgrade-e2e")
	}

	stack, candidateImage := startReleaseUpgradeStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	admin.createProject(t, "Upgrade", "upgrade")
	writer := admin.createServicePrincipal(t, "Upgrade writer", "upgrade-writer")
	admin.setMembership(t, "upgrade", writer, openapi.Writer)

	localTags := make([]string, 0, 4)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })
	variantA, variantB := buildFixtureImages(t, stack, &localTags)
	writerDocker := newDockerClient(t, stack, writer, &localTags)
	writerDocker.login(t, writer.username)
	firstTag := writerDocker.tag(t, variantA, "upgrade/app", "v1")
	writerDocker.push(t, firstTag)
	waitForRestartObservation(t, admin, "upgrade", "upgrade/app", []string{"v1"})

	upgradeReleaseStack(t, stack, candidateImage)

	upgradedAdmin := newManagementClient(t, stack.publicURL)
	upgradedAdmin.login(t)
	assertUpgradeProject(t, upgradedAdmin)
	assertUpgradeWriterAccess(t, upgradedAdmin, writer)
	waitForRestartObservation(t, upgradedAdmin, "upgrade", "upgrade/app", []string{"v1"})

	upgradedWriter := newDockerClient(t, stack, writer, &localTags)
	upgradedWriter.login(t, writer.username)
	removeLocalTag(t, stack.root, firstTag)
	upgradedWriter.pull(t, firstTag)
	secondTag := upgradedWriter.tag(t, variantB, "upgrade/app", "v2")
	upgradedWriter.push(t, secondTag)
	waitForRestartObservation(t, upgradedAdmin, "upgrade", "upgrade/app", []string{"v1", "v2"})

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	output, err := stack.compose(ctx, "restart", "grom", "distribution")
	cancel()
	if err != nil {
		t.Fatalf("restart upgraded services: %v\n%s", err, bounded(output))
	}
	waitForPublicStack(t, stack.publicURL)

	afterRestart := newManagementClient(t, stack.publicURL)
	afterRestart.login(t)
	assertUpgradeProject(t, afterRestart)
	assertUpgradeWriterAccess(t, afterRestart, writer)
	waitForRestartObservation(t, afterRestart, "upgrade", "upgrade/app", []string{"v1", "v2"})
	removeLocalTag(t, stack.root, firstTag)
	upgradedWriter.pull(t, firstTag)
}

func startReleaseUpgradeStack(t *testing.T) (*testStack, string) {
	t.Helper()
	requireDocker(t)
	baseImage := os.Getenv("GROM_UPGRADE_FROM_IMAGE")
	if baseImage == "" {
		baseImage = defaultUpgradeBaseImage
	}
	platform := os.Getenv("GROM_UPGRADE_PLATFORM")
	if platform == "" {
		platform = defaultUpgradePlatform
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved loopback port: %v", err)
	}

	root := repositoryRoot(t)
	project := os.Getenv("GROM_UPGRADE_COMPOSE_PROJECT")
	if project == "" {
		project = fmt.Sprintf("gromupgradee2e%d%d", os.Getpid(), time.Now().UnixNano())
	}
	candidateImage := "grom-registry-upgrade-" + project + ":candidate"
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	stack := &testStack{
		root:        root,
		composeFile: filepath.Join(root, "deploy", "compose", "docker-compose.yml"),
		project:     project,
		publicURL:   publicURL,
		registry:    strings.TrimPrefix(publicURL, "http://"),
		env: append(os.Environ(),
			"GROM_IMAGE="+baseImage,
			"DOCKER_DEFAULT_PLATFORM="+platform,
			"GROM_BIND_ADDRESS=127.0.0.1",
			fmt.Sprintf("GROM_HTTP_PORT=%d", port),
			"GROM_PUBLIC_URL="+publicURL,
			"GROM_DEPLOYMENT_PROFILE=development",
			"GROM_SECURE_COOKIES=false",
			"GROM_BOOTSTRAP_ADMIN_EMAIL="+e2eAdminEmail,
			"GROM_BOOTSTRAP_ADMIN_USERNAME=registry-e2e-admin",
			"GROM_BOOTSTRAP_ADMIN_PASSWORD="+e2eAdminPassword,
			"GROM_REGISTRY_TOKEN_TTL=5m",
			"GROM_AUTH_FAILURE_LIMIT=50",
			"GROM_AUTH_FAILURE_WINDOW=1m",
			"GROM_AUTH_BLOCK_DURATION=1m",
			"GROM_REGISTRY_HTTP_SECRET=registry-upgrade-e2e-http-secret",
		),
	}
	t.Cleanup(func() {
		if os.Getenv("GROM_KEEP_RELEASE_UPGRADE_STACK_ON_FAILURE") == "1" && t.Failed() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if output, downErr := stack.compose(ctx, "down", "--volumes", "--remove-orphans"); downErr != nil {
			t.Errorf("remove isolated Compose project: %v\n%s", downErr, bounded(output))
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), commandTimeout)
		defer cleanupCancel()
		_, _ = runCommand(cleanupCtx, root, nil, nil, "docker", "image", "rm", "--force", candidateImage)
	})

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	output, err := runCommand(ctx, root, nil, nil, "docker", "pull", "--platform", platform, baseImage)
	cancel()
	if err != nil {
		t.Fatalf("pull tagged upgrade base image %s: %v\n%s", baseImage, err, bounded(output))
	}
	ctx, cancel = context.WithTimeout(context.Background(), 6*time.Minute)
	output, err = runCommand(ctx, root, nil, nil,
		"docker", "build", "--platform", platform, "--pull=false", "--tag", candidateImage, root)
	cancel()
	if err != nil {
		t.Fatalf("build upgrade candidate image: %v\n%s", err, bounded(output))
	}
	ctx, cancel = context.WithTimeout(context.Background(), 6*time.Minute)
	output, err = stack.compose(ctx, "up", "--no-build", "--detach")
	cancel()
	if err != nil {
		t.Fatalf("start tagged base installation: %v\n%s", err, bounded(output))
	}
	waitForPublicStack(t, stack.publicURL)
	return stack, candidateImage
}

func upgradeReleaseStack(t *testing.T, stack *testStack, candidateImage string) {
	t.Helper()
	stack.env = replaceEnvironment(stack.env, "GROM_IMAGE", candidateImage)
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	output, err := stack.compose(ctx, "down", "--remove-orphans")
	cancel()
	if err != nil {
		t.Fatalf("stop tagged base installation for upgrade: %v\n%s", err, bounded(output))
	}
	ctx, cancel = context.WithTimeout(context.Background(), 6*time.Minute)
	output, err = stack.compose(ctx, "up", "--no-build", "--detach")
	cancel()
	if err != nil {
		t.Fatalf("start upgrade candidate installation: %v\n%s", err, bounded(output))
	}
	waitForPublicStack(t, stack.publicURL)
}

func replaceEnvironment(environment []string, key, value string) []string {
	prefix := key + "="
	updated := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			updated = append(updated, item)
		}
	}
	return append(updated, prefix+value)
}

func assertUpgradeProject(t *testing.T, api *managementClient) {
	t.Helper()
	var projects openapi.ProjectPage
	api.doJSON(t, http.MethodGet, "/api/v1/projects", nil, &projects, http.StatusOK)
	if len(projects.Items) != 1 || projects.Items[0].Slug != "upgrade" {
		t.Fatalf("upgraded projects = %#v", projects)
	}
}

func assertUpgradeWriterAccess(t *testing.T, api *managementClient, writer servicePrincipal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), managementRequestTimeout)
	defer cancel()
	token, _, err := api.exchangeToken(ctx, writer.username, writer.secret, "upgrade/app")
	if err != nil {
		t.Fatalf("exchange preserved writer access key: %v", err)
	}
	assertTokenAction(t, token.Token, "upgrade/app", "pull", true)
	assertTokenAction(t, token.Token, "upgrade/app", "push", true)
}
