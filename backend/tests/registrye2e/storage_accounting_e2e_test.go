package registrye2e

import (
	"os"
	"testing"
	"time"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

func TestStorageAccountingThroughPublicAPI(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}

	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	alpha := admin.createProject(t, "Storage Alpha", "storage-alpha")
	beta := admin.createProject(t, "Storage Beta", "storage-beta")
	assertPendingStorageUsage(t, alpha.AccountedUsage, "new alpha project")
	assertPendingStorageUsage(t, beta.AccountedUsage, "new beta project")

	writer := admin.createServicePrincipal(t, "Storage writer", "storage-writer")
	admin.setMembership(t, "storage-alpha", writer, openapi.Writer)
	admin.setMembership(t, "storage-beta", writer, openapi.Writer)

	localTags := make([]string, 0, 12)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })
	variantA, variantB := buildFixtureImages(t, stack, &localTags)
	writerDocker := newDockerClient(t, stack, writer, &localTags)
	writerDocker.login(t, writer.username)

	first := writerDocker.tag(t, variantA, "storage-alpha/app", "v1")
	writerDocker.push(t, first)
	alphaUsage := waitForReadyStorageUsage(t, admin, "storage-alpha", "app")
	baseUsage := alphaUsage.project
	if baseUsage <= 0 {
		t.Fatalf("first pushed image accounting = %d, want positive bytes", baseUsage)
	}
	if alphaUsage.repositories["app"] != baseUsage {
		t.Fatalf("first repository accounting = %d, want project accounting %d", alphaUsage.repositories["app"], baseUsage)
	}

	secondTag := writerDocker.tag(t, variantA, "storage-alpha/app", "v2")
	writerDocker.push(t, secondTag)
	alphaUsage = waitForReadyStorageUsage(t, admin, "storage-alpha", "app")
	assertStorageUsage(t, alphaUsage, baseUsage, map[string]int64{"app": baseUsage}, "same manifest under a second tag")

	sharedRepository := writerDocker.tag(t, variantA, "storage-alpha/worker", "v1")
	writerDocker.push(t, sharedRepository)
	alphaUsage = waitForReadyStorageUsage(t, admin, "storage-alpha", "app", "worker")
	assertStorageUsage(t, alphaUsage, baseUsage, map[string]int64{"app": baseUsage, "worker": baseUsage}, "same descriptors in a second repository")

	betaImage := writerDocker.tag(t, variantA, "storage-beta/app", "v1")
	writerDocker.push(t, betaImage)
	betaUsage := waitForReadyStorageUsage(t, admin, "storage-beta", "app")
	assertStorageUsage(t, betaUsage, baseUsage, map[string]int64{"app": baseUsage}, "same descriptors in a second project")

	additionalImage := writerDocker.tag(t, variantB, "storage-alpha/app", "v3")
	writerDocker.push(t, additionalImage)
	alphaUsage = waitForReadyStorageUsage(t, admin, "storage-alpha", "app", "worker")
	if alphaUsage.project <= baseUsage {
		t.Fatalf("project accounting after distinct image = %d, want greater than %d", alphaUsage.project, baseUsage)
	}

	admin.reconcileInventory(t, "storage-alpha", "app")
	alphaUsage = waitForReadyStorageUsage(t, admin, "storage-alpha", "app", "worker")
	if alphaUsage.project <= baseUsage {
		t.Fatalf("project accounting after reconciliation = %d, want greater than %d", alphaUsage.project, baseUsage)
	}

	admin.deleteArtifact(t, "storage-alpha", "app", "v3")
	alphaUsage = waitForReadyStorageUsage(t, admin, "storage-alpha", "app", "worker")
	assertStorageUsage(t, alphaUsage, baseUsage, map[string]int64{"app": baseUsage, "worker": baseUsage}, "deleting the distinct image")
}

type storageUsageSnapshot struct {
	project      int64
	repositories map[string]int64
}

func waitForReadyStorageUsage(
	t *testing.T,
	api *managementClient,
	projectSlug string,
	repositoryNames ...string,
) storageUsageSnapshot {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		project := api.project(t, projectSlug)
		repositories := api.repositories(t, projectSlug)
		usage, ready := readyStorageUsage(project, repositories, repositoryNames)
		if ready {
			return usage
		}
		if time.Now().After(deadline) {
			t.Fatalf("storage accounting did not become ready for project %q: project=%#v repositories=%#v", projectSlug, project.AccountedUsage, repositories)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func readyStorageUsage(
	project openapi.Project,
	repositories []openapi.Repository,
	repositoryNames []string,
) (storageUsageSnapshot, bool) {
	if project.AccountedUsage.Status != openapi.AccountedStorageUsageStatusReady || project.AccountedUsage.AccountedBytes == nil {
		return storageUsageSnapshot{}, false
	}
	requested := make(map[string]struct{}, len(repositoryNames))
	for _, name := range repositoryNames {
		requested[name] = struct{}{}
	}
	usage := storageUsageSnapshot{project: *project.AccountedUsage.AccountedBytes, repositories: make(map[string]int64, len(requested))}
	for _, repository := range repositories {
		if _, wanted := requested[repository.Name]; !wanted {
			continue
		}
		if repository.AccountedUsage.Status != openapi.AccountedStorageUsageStatusReady || repository.AccountedUsage.AccountedBytes == nil {
			return storageUsageSnapshot{}, false
		}
		usage.repositories[repository.Name] = *repository.AccountedUsage.AccountedBytes
	}
	return usage, len(usage.repositories) == len(requested)
}

func assertPendingStorageUsage(t *testing.T, usage openapi.AccountedStorageUsage, subject string) {
	t.Helper()
	if usage.Status != openapi.AccountedStorageUsageStatusPending || usage.AccountedBytes != nil || usage.ReconciledAt != nil {
		t.Fatalf("%s accounting = %#v, want pending without bytes or reconciliation time", subject, usage)
	}
}

func assertStorageUsage(
	t *testing.T,
	actual storageUsageSnapshot,
	wantProject int64,
	wantRepositories map[string]int64,
	operation string,
) {
	t.Helper()
	if actual.project != wantProject {
		t.Fatalf("project accounting after %s = %d, want %d", operation, actual.project, wantProject)
	}
	for repository, want := range wantRepositories {
		if got := actual.repositories[repository]; got != want {
			t.Fatalf("repository %q accounting after %s = %d, want %d", repository, operation, got, want)
		}
	}
}
