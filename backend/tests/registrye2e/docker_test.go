package registrye2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type dockerClient struct {
	configDir  string
	registry   string
	root       string
	localTags  *[]string
	credential string
}

func newDockerClient(
	t *testing.T,
	stack *testStack,
	principal servicePrincipal,
	localTags *[]string,
) *dockerClient {
	t.Helper()
	client := &dockerClient{
		configDir: t.TempDir(), registry: stack.registry, root: stack.root,
		localTags: localTags, credential: principal.secret,
	}
	return client
}

func (d *dockerClient) login(t *testing.T, username string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, d.root, d.environment(), strings.NewReader(d.credential+"\n"),
		"docker", "login", "--username", username, "--password-stdin", d.registry)
	output = strings.ReplaceAll(output, d.credential, "[REDACTED]")
	if err != nil {
		t.Fatalf("docker login to isolated registry failed: %v\n%s", err, bounded(output))
	}
}

func buildFixtureImages(t *testing.T, stack *testStack, localTags *[]string) (string, string) {
	t.Helper()
	variantA := "grom-registry-e2e-" + stack.project + "-a:source"
	variantB := "grom-registry-e2e-" + stack.project + "-b:source"
	for _, fixture := range []struct {
		tag  string
		path string
	}{
		{variantA, filepath.Join(stack.root, "backend", "tests", "registrye2e", "fixtures", "variant-a")},
		{variantB, filepath.Join(stack.root, "backend", "tests", "registrye2e", "fixtures", "variant-b")},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		output, err := runCommand(ctx, stack.root, nil, nil,
			"docker", "build", "--pull=false", "--tag", fixture.tag, fixture.path)
		cancel()
		if err != nil {
			t.Fatalf("build network-independent %s image: %v\n%s", filepath.Base(fixture.path), err, bounded(output))
		}
		*localTags = append(*localTags, fixture.tag)
	}
	return variantA, variantB
}

func (d *dockerClient) tag(t *testing.T, source, repository, tag string) string {
	t.Helper()
	target := fmt.Sprintf("%s/%s:%s", d.registry, repository, tag)
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, d.root, nil, nil, "docker", "tag", source, target)
	if err != nil {
		t.Fatalf("tag isolated E2E image as %s: %v\n%s", target, err, bounded(output))
	}
	*d.localTags = append(*d.localTags, target)
	return target
}

func (d *dockerClient) push(t *testing.T, target string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, d.root, d.environment(), nil, "docker", "push", target)
	if err != nil {
		t.Fatalf("docker push %s failed: %v\n%s", target, err, bounded(output))
	}
	return output
}

func (d *dockerClient) pushDenied(t *testing.T, target string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, d.root, d.environment(), nil, "docker", "push", target)
	if err == nil {
		t.Fatalf("docker push %s unexpectedly succeeded", target)
	}
	return bounded(output)
}

func (d *dockerClient) pull(t *testing.T, target string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, d.root, d.environment(), nil, "docker", "pull", target)
	if err != nil {
		t.Fatalf("docker pull %s failed: %v\n%s", target, err, bounded(output))
	}
	return output
}

func (d *dockerClient) pullDenied(t *testing.T, target string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, d.root, d.environment(), nil, "docker", "pull", target)
	if err == nil {
		t.Fatalf("docker pull %s unexpectedly succeeded", target)
	}
	return bounded(output)
}

func removeLocalTag(t *testing.T, root, tag string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	_, _ = runCommand(ctx, root, nil, nil, "docker", "image", "rm", tag)
}

func cleanupLocalTags(t *testing.T, root string, tags []string) {
	t.Helper()
	for index := len(tags) - 1; index >= 0; index-- {
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		output, err := runCommand(ctx, root, nil, nil, "docker", "image", "rm", "--force", tags[index])
		cancel()
		if err != nil && !strings.Contains(strings.ToLower(output), "no such image") {
			t.Errorf("remove isolated image tag %s: %v\n%s", tags[index], err, bounded(output))
		}
	}
}

func (d *dockerClient) environment() []string {
	return environmentWith("DOCKER_CONFIG", d.configDir)
}

func environmentWith(key, value string) []string {
	prefix := key + "="
	environment := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, prefix) {
			environment = append(environment, item)
		}
	}
	return append(environment, prefix+value)
}
