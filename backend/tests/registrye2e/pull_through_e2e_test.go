package registrye2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

func TestPullThroughJourney(t *testing.T) {
	if os.Getenv("GROM_RUN_PULL_THROUGH_E2E") != "1" {
		t.Skip("set GROM_RUN_PULL_THROUGH_E2E=1 or run make test-pull-through-e2e")
	}

	stack := startTestStack(t)
	t.Setenv("DOCKER_DEFAULT_PLATFORM", "linux/amd64")
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	localTags := make([]string, 0, 12)
	t.Cleanup(func() { cleanupLocalTags(t, stack.root, localTags) })

	for _, test := range []struct {
		name       string
		slug       string
		repository string
		tag        string
		upstream   openapi.CreateProjectRequestUpstream
	}{
		{name: "Docker Hub library image", slug: "dockerhub", repository: "hello-world", tag: "latest", upstream: openapi.CreateProjectRequestUpstreamDockerhub},
		{name: "GHCR nested image", slug: "ghcr", repository: "jfxdev/grom-registry", tag: "v1.0.6", upstream: openapi.CreateProjectRequestUpstreamGhcr},
	} {
		t.Run(test.name, func(t *testing.T) {
			mode := openapi.CreateProjectRequestModePullThrough
			upstream := test.upstream
			var project openapi.Project
			admin.doJSON(t, "POST", "/api/v1/projects", openapi.CreateProjectRequest{
				Name: test.name, Slug: test.slug, Mode: &mode, Upstream: &upstream,
			}, &project, 201)
			if project.Mode != openapi.ProjectModePullThrough || project.Upstream == nil || *project.Upstream != openapi.ProjectUpstream(test.upstream) {
				t.Fatalf("unexpected pull-through project: %#v", project)
			}
			principal := admin.createServicePrincipal(t, test.name+" reader", test.slug+"-reader")
			admin.setMembership(t, test.slug, principal, openapi.Admin)
			docker := newDockerClient(t, stack, principal, &localTags)
			target := fmt.Sprintf("%s/%s/%s:%s", stack.registry, test.slug, test.repository, test.tag)
			localTags = append(localTags, target)

			docker.login(t, principal.username)
			firstDigest := docker.pull(t, target)
			waitForPullThroughObservation(t, admin, test.slug, test.repository, test.tag, firstDigest)

			digestTarget := fmt.Sprintf("%s/%s/%s@%s", stack.registry, test.slug, test.repository, firstDigest)
			localTags = append(localTags, digestTarget)
			docker.login(t, principal.username)
			if digest := docker.pull(t, digestTarget); digest != firstDigest {
				t.Fatalf("digest pull changed content: first=%s digest=%s", firstDigest, digest)
			}

			removeLocalTag(t, stack.root, target)
			docker.login(t, principal.username)
			if secondDigest := docker.pull(t, target); secondDigest != firstDigest {
				t.Fatalf("cached tag changed digest: first=%s second=%s", firstDigest, secondDigest)
			}

			pushTarget := docker.tag(t, target, test.slug+"/"+test.repository, "push-denied")
			docker.login(t, principal.username)
			assertRegistryDenied(t, docker.pushDenied(t, pushTarget))

			ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
			output, err := stack.compose(ctx, "restart", "grom", "distribution")
			cancel()
			if err != nil {
				t.Fatalf("restart pull-through stack: %v\n%s", err, bounded(output))
			}
			waitForPublicStack(t, stack.publicURL)
			removeLocalTag(t, stack.root, target)
			freshDocker := newDockerClient(t, stack, principal, &localTags)
			freshDocker.login(t, principal.username)
			if digest := freshDocker.pull(t, target); digest != firstDigest {
				t.Fatalf("restart changed cached digest: first=%s after=%s", firstDigest, digest)
			}
			postRestartAdmin := newManagementClient(t, stack.publicURL)
			postRestartAdmin.login(t)
			waitForPullThroughObservation(t, postRestartAdmin, test.slug, test.repository, test.tag, firstDigest)
		})
	}
}

func waitForPullThroughObservation(t *testing.T, api *managementClient, project, repository, tag, digest string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		repositories := api.repositories(t, project)
		tags := api.tags(t, project, repository)
		inventory := api.inventoryQuery(t, project, repository, digest)
		repositoryReady := false
		for _, candidate := range repositories {
			if candidate.Name == repository && candidate.CreationSource == openapi.RepositoryCreationSourcePullThrough &&
				candidate.Status == openapi.RepositoryStatusActive {
				repositoryReady = true
				break
			}
		}
		manifestReady := false
		for _, manifest := range inventory {
			if manifest.Digest == digest && contains(manifest.Tags, tag) {
				manifestReady = true
				break
			}
		}
		if repositoryReady && contains(tags.Items, tag) && manifestReady {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pull-through observation did not converge: repositories=%#v tags=%#v inventory=%#v", repositories, tags, inventory)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
