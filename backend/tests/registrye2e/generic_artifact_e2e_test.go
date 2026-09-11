package registrye2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

// openTofuModuleArtifactType is what ORAS-based publishing tools stamp on a
// packaged OpenTofu module.
const openTofuModuleArtifactType = "application/vnd.opentofu.modulepkg"

type orasClient struct {
	registry  string
	configDir string
	root      string
	username  string
	secret    string
}

func requireOras(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("oras"); err != nil {
		t.Fatalf("the generic artifact journey requires the ORAS CLI: %v", err)
	}
}

func newOrasClient(t *testing.T, stack *testStack, principal servicePrincipal) *orasClient {
	t.Helper()
	client := &orasClient{
		registry: stack.registry, configDir: t.TempDir(), root: stack.root,
		username: principal.username, secret: principal.secret,
	}
	client.login(t)
	return client
}

// login writes credentials to this journey's own --registry-config file rather
// than the shared Docker credential store, matching the Docker journey's
// isolation rules.
func (o *orasClient) login(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, o.root, nil, strings.NewReader(o.secret+"\n"), "oras", "login",
		"--registry-config", filepath.Join(o.configDir, "oras.json"),
		"--plain-http", "--username", o.username, "--password-stdin", o.registry)
	output = strings.ReplaceAll(output, o.secret, "[REDACTED]")
	if err != nil {
		t.Fatalf("oras login to the isolated registry failed: %v\n%s", err, bounded(output))
	}
}

// writeModulePackage writes a deterministic stand-in for a packaged OpenTofu
// module and returns its directory and file name.
func writeModulePackage(t *testing.T, name, contents string) (string, string) {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return directory, name
}

func (o *orasClient) pushArtifact(t *testing.T, reference, artifactType, directory, file string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, directory, nil, nil, "oras", "push",
		"--registry-config", filepath.Join(o.configDir, "oras.json"),
		"--plain-http", "--artifact-type", artifactType,
		o.registry+"/"+reference, file+":archive/tar+gzip")
	output = strings.ReplaceAll(output, o.secret, "[REDACTED]")
	if err != nil {
		t.Fatalf("oras push %s failed: %v\n%s", reference, err, bounded(output))
	}
}

func (o *orasClient) attach(t *testing.T, reference, artifactType, directory, file string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, directory, nil, nil, "oras", "attach",
		"--registry-config", filepath.Join(o.configDir, "oras.json"),
		"--plain-http", "--artifact-type", artifactType,
		o.registry+"/"+reference, file+":application/json")
	output = strings.ReplaceAll(output, o.secret, "[REDACTED]")
	if err != nil {
		t.Fatalf("oras attach %s failed: %v\n%s", reference, err, bounded(output))
	}
}

func (o *orasClient) pull(t *testing.T, reference string) string {
	t.Helper()
	target := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := runCommand(ctx, target, nil, nil, "oras", "pull",
		"--registry-config", filepath.Join(o.configDir, "oras.json"),
		"--plain-http", "--output", target, o.registry+"/"+reference)
	if err != nil {
		t.Fatalf("oras pull %s failed: %v\n%s", reference, err, bounded(output))
	}
	return target
}

// TestGenericOCIArtifactJourney proves Grom serves arbitrary OCI artifacts, not
// only container images: a real ORAS client publishes an OpenTofu module into a
// repository that does not exist yet, the control plane classifies and measures
// it, a fresh credential directory pulls it back byte for byte, and an attached
// referrer is inventoried without changing the repository's profile.
func TestGenericOCIArtifactJourney(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}
	requireOras(t)

	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	admin.createProject(t, "Platform modules", "platform-modules")
	writer := admin.createServicePrincipal(t, "Module publisher", "module-publisher")
	admin.setMembership(t, "platform-modules", writer, openapi.Writer)

	const repository = "platform-modules/vpc"
	client := newOrasClient(t, stack, writer)

	moduleBody := "opentofu-module-fixture\nvariable \"cidr\" {}\n"
	directory, file := writeModulePackage(t, "module.tgz", moduleBody)

	// The repository does not exist yet: the first push must create it.
	client.pushArtifact(t, repository+":1.0.0", openTofuModuleArtifactType, directory, file)

	repositories := admin.repositories(t, "platform-modules")
	var module *openapi.Repository
	for index := range repositories {
		if repositories[index].Name == "vpc" {
			module = &repositories[index]
		}
	}
	if module == nil {
		t.Fatalf("the first artifact push must create the logical repository, got %+v", repositories)
	}
	if module.CreationSource != "push" {
		t.Fatalf("expected the repository to record a push creation source, got %q", module.CreationSource)
	}
	if module.Profile != openapi.RepositoryProfileOpentofuModule {
		t.Fatalf("expected the OpenTofu module profile, got %q", module.Profile)
	}
	if module.ProfileNeedsReview {
		t.Fatal("a single artifact type must not require profile review")
	}

	inventory := admin.inventory(t, "platform-modules", "vpc")
	primary := findPrimaryManifest(t, inventory)
	if primary.ArtifactType == nil || *primary.ArtifactType != openTofuModuleArtifactType {
		t.Fatalf("expected the artifact type to be preserved verbatim, got %+v", primary.ArtifactType)
	}
	if primary.ObservedKind != openapi.ArtifactKindOpentofuModule {
		t.Fatalf("expected the OpenTofu module kind, got %q", primary.ObservedKind)
	}
	// Without the leaf-manifest measurement an artifact with no image config
	// reports nothing at all, which is what this assertion guards.
	if totalCompressedSize(primary) < int64(len(moduleBody)) {
		t.Fatalf("expected the artifact's content to be measured, got %+v", primary.Platforms)
	}
	for _, platform := range primary.Platforms {
		if platform.Os != "" || platform.Architecture != "" {
			t.Fatalf("an OpenTofu module must not claim a platform, got %+v", platform)
		}
	}

	// A fresh credential directory proves the pull path does not depend on
	// state left behind by the push.
	reader := newOrasClient(t, stack, writer)
	pulled := reader.pull(t, repository+":1.0.0")
	roundTripped, err := os.ReadFile(filepath.Join(pulled, file))
	if err != nil {
		t.Fatalf("read the pulled module package: %v", err)
	}
	if string(roundTripped) != moduleBody {
		t.Fatalf("the pulled module package differs from the pushed one: %q", string(roundTripped))
	}
	// An OCI 1.1 referrer describes the module rather than the repository, so it
	// must be inventoried without moving the profile.
	sbomDirectory, sbomFile := writeModulePackage(t, "sbom.json", `{"bomFormat":"CycloneDX","specVersion":"1.5"}`)
	client.attach(t, repository+":1.0.0", "application/vnd.cyclonedx+json", sbomDirectory, sbomFile)

	admin.reconcileInventory(t, "platform-modules", "vpc")
	afterAttach := admin.repositories(t, "platform-modules")
	for _, candidate := range afterAttach {
		if candidate.Name != "vpc" {
			continue
		}
		if candidate.Profile != openapi.RepositoryProfileOpentofuModule {
			t.Fatalf("a referrer must not change the repository profile, got %q", candidate.Profile)
		}
	}
	referrers := 0
	for _, manifest := range admin.inventory(t, "platform-modules", "vpc") {
		if manifest.ArtifactRelationship == openapi.Referrer {
			referrers++
		}
	}
	if referrers == 0 {
		t.Fatal("expected the attached referrer to be inventoried")
	}
}

// TestRegistryTokenAcceptsSpaceDelimitedScopes covers the scope form oras-go
// uses whenever a command touches more than one repository, such as oras cp.
func TestRegistryTokenAcceptsSpaceDelimitedScopes(t *testing.T) {
	if os.Getenv("GROM_RUN_REGISTRY_E2E") != "1" {
		t.Skip("set GROM_RUN_REGISTRY_E2E=1 or run make test-registry-e2e")
	}

	stack := startTestStack(t)
	admin := newManagementClient(t, stack.publicURL)
	admin.login(t)
	admin.createProject(t, "Multi scope", "multi-scope")
	writer := admin.createServicePrincipal(t, "Multi scope writer", "multi-scope-writer")
	admin.setMembership(t, "multi-scope", writer, openapi.Writer)

	query := url.Values{
		"service": {"grom-registry"},
		"scope":   {"repository:multi-scope/first:pull,push repository:multi-scope/second:pull"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), managementRequestTimeout)
	defer cancel()
	token, status, err := admin.exchangeTokenWithQuery(ctx, writer.username, writer.secret, query)
	if err != nil {
		t.Fatalf("space-delimited scope exchange returned %d: %v", status, err)
	}

	access := decodeAccessClaims(t, token.Token)
	if len(access) != 2 {
		t.Fatalf("expected both scopes to be granted, got %+v", access)
	}
	if access[0].Name != "multi-scope/first" || !containsString(access[0].Actions, "push") {
		t.Fatalf("expected push on the first repository, got %+v", access[0])
	}
	if access[1].Name != "multi-scope/second" || !containsString(access[1].Actions, "pull") {
		t.Fatalf("expected pull on the second repository, got %+v", access[1])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findPrimaryManifest(t *testing.T, inventory []openapi.ManifestInventory) openapi.ManifestInventory {
	t.Helper()
	for _, manifest := range inventory {
		if manifest.ArtifactRelationship == openapi.Primary && len(manifest.Tags) > 0 {
			return manifest
		}
	}
	t.Fatalf("no tagged primary manifest in inventory: %+v", inventory)
	return openapi.ManifestInventory{}
}

func totalCompressedSize(manifest openapi.ManifestInventory) int64 {
	var total int64
	for _, platform := range manifest.Platforms {
		total += platform.CompressedSize
	}
	return total
}

type registryAccess struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// decodeAccessClaims reads the access list out of a signed registry token. The
// signature is Distribution's to verify; this journey only asserts which scopes
// Grom granted.
func decodeAccessClaims(t *testing.T, token string) []registryAccess {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a three-part JWT, got %d parts", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode registry token payload: %v", err)
	}
	var claims struct {
		Access []registryAccess `json:"access"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("decode registry token claims: %v", err)
	}
	return claims.Access
}
