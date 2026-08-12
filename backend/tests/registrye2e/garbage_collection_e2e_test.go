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

	localTags := make([]string, 0, 6)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })
	variantA, variantB := buildFixtureImages(t, stack, &localTags)
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

	// Distribution caches blob existence in-process. The maintenance supervisor
	// must restart it around GC so publishing the exact same digest uploads the
	// reclaimed content instead of leaving a dangling tag link.
	writerDocker.push(t, target)
	waitForRestartObservation(t, admin, "gc-alpha", "gc-alpha/app", []string{"v1"})
	freshPuller := newDockerClient(t, stack, writer, &localTags)
	freshPuller.login(t, writer.username)
	removeLocalTag(t, stack.root, target)
	freshPuller.pull(t, target)

	admin.deleteArtifact(t, "gc-alpha", "app", "v1")
	secondResult := admin.garbageCollect(t)
	if secondResult.ReclaimedBytes <= 0 || secondResult.BytesAfter >= secondResult.BytesBefore {
		t.Fatalf("second garbage collection did not reclaim republished content: %#v", secondResult)
	}

	// Reusing the same test tag for a different digest must also become visible
	// immediately through both the registry and Grom's reconciled inventory.
	replacement := writerDocker.tag(t, variantB, "gc-alpha/app", "v1")
	replacementDigest := writerDocker.push(t, replacement)
	waitForRestartObservation(t, admin, "gc-alpha", "gc-alpha/app", []string{"v1"})
	removeLocalTag(t, stack.root, replacement)
	pulledDigest := freshPuller.pull(t, replacement)
	if pulledDigest != replacementDigest {
		t.Fatalf("replacement v1 digest = %s after pull, want %s", pulledDigest, replacementDigest)
	}
}
