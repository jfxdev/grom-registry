package registrye2e

import (
	"os"
	"testing"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

func TestGarbageCollectionReclaimsDeletedManifestStorage(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}
	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	admin.createProject(t, "GC Alpha", "gc-alpha")
	writer := admin.createServicePrincipal(t, "GC writer", "gc-writer")
	admin.setMembership(t, "gc-alpha", writer, openapi.Writer)

	localTags := make([]string, 0, 2)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })
	variantA, _ := buildFixtureImages(t, stack, &localTags)
	writerDocker := newDockerClient(t, stack, writer, &localTags)
	writerDocker.login(t, writer.username)
	target := writerDocker.tag(t, variantA, "gc-alpha/app", "v1")
	writerDocker.push(t, target)
	waitForRestartObservation(t, admin, "gc-alpha", "gc-alpha/app", []string{"v1"})

	admin.deleteArtifact(t, "gc-alpha", "app", "v1")
	result := admin.garbageCollect(t)
	if result.ReclaimedBytes <= 0 || result.BytesAfter >= result.BytesBefore {
		t.Fatalf("garbage collection did not reclaim deleted content: %#v", result)
	}
}
