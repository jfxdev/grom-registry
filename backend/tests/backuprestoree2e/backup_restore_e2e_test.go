package backuprestoree2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	openapi "github.com/jfxdev/grom/backend/internal/generated/openapi"
)

const (
	adminEmail     = "backup-restore-e2e@grom.local"
	adminPassword  = "backup-restore-e2e-password"
	commandTimeout = 3 * time.Minute
)

func TestBackupRestoreJourney(t *testing.T) {
	if os.Getenv("GROM_RUN_BACKUP_RESTORE_E2E") != "1" {
		t.Skip("set GROM_RUN_BACKUP_RESTORE_E2E=1 or run make test-backup-restore-e2e")
	}
	requireDocker(t)
	root := repositoryRoot(t)
	port := reservePort(t)
	recoveryPort := reservePort(t)
	project := fmt.Sprintf("grombackupe2e%d%d", os.Getpid(), time.Now().UnixNano())
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	registry := strings.TrimPrefix(publicURL, "http://")
	env := append(os.Environ(),
		"GROM_BIND_ADDRESS=127.0.0.1", fmt.Sprintf("GROM_HTTP_PORT=%d", port),
		"GROM_PUBLIC_URL="+publicURL, "GROM_DEPLOYMENT_PROFILE=development",
		"GROM_SECURE_COOKIES=false", "GROM_BOOTSTRAP_ADMIN_EMAIL="+adminEmail,
		"GROM_BOOTSTRAP_ADMIN_USERNAME=backup-admin", "GROM_BOOTSTRAP_ADMIN_PASSWORD="+adminPassword,
		"GROM_REGISTRY_HTTP_SECRET=backup-restore-e2e-secret", "GROM_AUTH_FAILURE_LIMIT=50",
		fmt.Sprintf("GROM_RECOVERY_PORT=%d", recoveryPort), "GROM_VERSION=dev",
	)
	stack := composeStack{root: root, project: project, env: env}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_, _ = stack.compose(ctx, "down", "--volumes", "--remove-orphans")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	if output, err := stack.compose(ctx, "up", "--build", "--detach"); err != nil {
		t.Fatalf("start source installation: %v\n%s", err, bounded(output))
	}
	waitReady(t, publicURL)

	api := newAPIClient(t, publicURL)
	api.login(t)
	api.doJSON(t, http.MethodPost, "/api/v1/projects",
		openapi.CreateProjectRequest{Name: "Recovery", Slug: "recovery"}, nil, http.StatusCreated)
	var account openapi.ServiceAccount
	api.doJSON(t, http.MethodPost, "/api/v1/service-accounts",
		openapi.CreateServiceAccountRequest{Name: "Recovery writer", Username: "recovery-writer"},
		&account, http.StatusCreated)
	var created openapi.CreatedToken
	api.doJSON(t, http.MethodPost, "/api/v1/service-accounts/"+url.PathEscape(account.Id.String())+"/tokens",
		openapi.CreateServiceAccountTokenRequest{Name: "recovery"}, &created, http.StatusCreated)
	membershipPath := fmt.Sprintf("/api/v1/projects/recovery/members/service_account/%s", url.PathEscape(account.Id.String()))
	api.doJSON(t, http.MethodPut, membershipPath,
		openapi.SetMembershipRequest{Role: openapi.Writer}, nil, http.StatusOK)

	sourceTag := "grom-backup-e2e-" + project + ":source"
	firstTag := registry + "/recovery/app:v1"
	secondTag := registry + "/recovery/app:v2"
	t.Cleanup(func() {
		for _, tag := range []string{secondTag, firstTag, sourceTag} {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), commandTimeout)
			_, _ = run(cleanupCtx, root, nil, nil, "docker", "image", "rm", "--force", tag)
			cleanupCancel()
		}
	})
	fixture := filepath.Join(root, "backend", "tests", "registrye2e", "fixtures", "variant-a")
	mustRun(t, root, nil, nil, "docker", "build", "--pull=false", "--tag", sourceTag, fixture)
	mustRun(t, root, nil, nil, "docker", "tag", sourceTag, firstTag)
	dockerConfig := t.TempDir()
	dockerEnv := withEnv(env, "DOCKER_CONFIG", dockerConfig)
	mustRun(t, root, dockerEnv, strings.NewReader(created.Secret+"\n"),
		"docker", "login", "--username", account.Username, "--password-stdin", registry)
	mustRun(t, root, dockerEnv, nil, "docker", "push", firstTag)
	sourceSigningKeyID := api.signingKeyID(t, account.Username, created.Secret, "recovery/app")

	backupID := api.createAndWaitForBackup(t)
	bundle := api.downloadBackup(t, backupID)
	if len(bundle) == 0 {
		t.Fatal("backup download was empty")
	}
	api.doJSON(t, http.MethodDelete, "/api/v1/backups/"+url.PathEscape(backupID), nil, nil, http.StatusNoContent)
	var afterDeletion openapi.BackupOverview
	api.doJSON(t, http.MethodGet, "/api/v1/backups", nil, &afterDeletion, http.StatusOK)
	if afterDeletion.TotalBackups != 0 || len(afterDeletion.Backups) != 0 {
		t.Fatalf("deleted backup remains listed: %#v", afterDeletion)
	}
	api.doJSON(t, http.MethodDelete, "/api/v1/backups/"+url.PathEscape(backupID), nil, nil, http.StatusNotFound)

	if output, err := stack.compose(ctx, "down", "--volumes", "--remove-orphans"); err != nil {
		t.Fatalf("destroy original installation volumes: %v\n%s", err, bounded(output))
	}
	if output, err := stack.compose(ctx, "--profile", "recovery", "up", "--detach", "--no-build", "recovery"); err != nil {
		t.Fatalf("start recovery UI: %v\n%s", err, bounded(output))
	}
	recoveryURL := fmt.Sprintf("http://127.0.0.1:%d", recoveryPort)
	token := waitForRecoveryUI(t, recoveryURL)
	recoveryUpload(t, recoveryURL, token, bundle, http.StatusCreated)
	recoveryRestore(t, recoveryURL, token, backupID)
	if output, err := stack.compose(ctx, "--profile", "recovery", "stop", "recovery"); err != nil {
		t.Fatalf("stop recovery UI: %v\n%s", err, bounded(output))
	}
	if output, err := stack.compose(ctx, "up", "--detach", "--no-build", "grom", "distribution", "backup-agent"); err != nil {
		logs, _ := stack.compose(ctx, "logs", "--no-color", "--tail", "120", "grom", "distribution")
		t.Fatalf("start restored installation: %v\n%s\n%s", err, bounded(output), bounded(logs))
	}
	waitReady(t, publicURL)

	api.doJSON(t, http.MethodGet, "/api/v1/me", nil, nil, http.StatusUnauthorized)
	api = newAPIClient(t, publicURL)
	api.login(t)
	var projects []openapi.Project
	api.doJSON(t, http.MethodGet, "/api/v1/projects", nil, &projects, http.StatusOK)
	if len(projects) != 1 || projects[0].Slug != "recovery" {
		t.Fatalf("restored projects = %#v", projects)
	}
	if restoredKeyID := api.signingKeyID(t, account.Username, created.Secret, "recovery/app"); restoredKeyID != sourceSigningKeyID {
		t.Fatalf("restored signing key ID = %q, want %q", restoredKeyID, sourceSigningKeyID)
	}

	mustRun(t, root, nil, nil, "docker", "image", "rm", "--force", firstTag)
	mustRun(t, root, dockerEnv, nil, "docker", "pull", firstTag)
	mustRun(t, root, nil, nil, "docker", "tag", sourceTag, secondTag)
	mustRun(t, root, dockerEnv, nil, "docker", "push", secondTag)

	corruptBundle := append([]byte(nil), bundle...)
	corruptBundle[len(corruptBundle)/2] ^= 0xff
	corruptProject := project + "corrupt"
	corruptRecoveryPort := reservePort(t)
	corruptStack := composeStack{
		root: root, project: corruptProject,
		env: withEnv(stack.env, "GROM_RECOVERY_PORT", fmt.Sprintf("%d", corruptRecoveryPort)),
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_, _ = corruptStack.compose(cleanupCtx, "down", "--volumes", "--remove-orphans")
	})
	if output, err := corruptStack.compose(ctx, "--profile", "recovery", "up", "--detach", "--no-build", "recovery"); err != nil {
		t.Fatalf("start corruption recovery UI: %v\n%s", err, bounded(output))
	}
	corruptURL := fmt.Sprintf("http://127.0.0.1:%d", corruptRecoveryPort)
	corruptToken := waitForRecoveryUI(t, corruptURL)
	recoveryUpload(t, corruptURL, corruptToken, corruptBundle, http.StatusBadRequest)
	if response, err := http.Get(corruptURL + "/api/backups"); err != nil {
		t.Fatal(err)
	} else {
		defer response.Body.Close()
		var backups []any
		_ = json.NewDecoder(response.Body).Decode(&backups)
		if len(backups) != 0 {
			t.Fatal("corrupt upload was published")
		}
	}
}

type composeStack struct {
	root, project string
	env           []string
}

func (stack composeStack) compose(ctx context.Context, arguments ...string) (string, error) {
	args := []string{"compose", "--ansi", "never", "--project-name", stack.project,
		"--file", filepath.Join(stack.root, "deploy", "compose", "docker-compose.yml"),
		"--file", filepath.Join(stack.root, "deploy", "backup", "docker-compose.backup.yml")}
	args = append(args, arguments...)
	return run(ctx, stack.root, stack.env, nil, "docker", args...)
}

type apiClient struct {
	baseURL string
	client  *http.Client
}

func newAPIClient(t *testing.T, baseURL string) *apiClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &apiClient{baseURL: baseURL, client: &http.Client{Jar: jar, Timeout: 15 * time.Second}}
}

func (client *apiClient) login(t *testing.T) {
	t.Helper()
	client.doJSON(t, http.MethodPost, "/api/v1/session",
		openapi.LoginRequest{Email: adminEmail, Password: adminPassword}, nil, http.StatusOK)
}

func (client *apiClient) createAndWaitForBackup(t *testing.T) string {
	t.Helper()
	var operation openapi.BackupOperation
	client.doJSON(t, http.MethodPost, "/api/v1/backups", nil, &operation, http.StatusAccepted)
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var overview openapi.BackupOverview
		client.doJSON(t, http.MethodGet, "/api/v1/backups", nil, &overview, http.StatusOK)
		if overview.ActiveOperation != nil {
			switch overview.ActiveOperation.Status {
			case openapi.BackupStatusFailed:
				t.Fatalf("integrated backup failed: %v", overview.ActiveOperation.Message)
			case openapi.BackupStatusComplete:
				if len(overview.Backups) != 1 {
					t.Fatalf("completed backup overview = %#v", overview)
				}
				return overview.Backups[0].BackupId.String()
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("integrated backup did not complete")
	return ""
}

func (client *apiClient) downloadBackup(t *testing.T, backupID string) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, client.baseURL+"/api/v1/backups/"+url.PathEscape(backupID)+"/download", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		t.Fatalf("download backup: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/x-tar" {
		t.Fatalf("download backup returned HTTP %d and type %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read backup download: %v", err)
	}
	return raw
}

func (client *apiClient) signingKeyID(t *testing.T, username, secret, repository string) string {
	t.Helper()
	query := url.Values{
		"service": {"grom-registry"},
		"scope":   {"repository:" + repository + ":pull,push"},
	}
	request, err := http.NewRequest(http.MethodGet, client.baseURL+"/auth/token?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth(username, secret)
	response, err := client.client.Do(request)
	if err != nil {
		t.Fatalf("exchange registry token: %v", err)
	}
	defer response.Body.Close()
	var token openapi.RegistryToken
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&token) != nil {
		t.Fatalf("exchange registry token returned HTTP %d", response.StatusCode)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(token.Token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse registry JWT: %v", err)
	}
	keyID, _ := parsed.Header["kid"].(string)
	if keyID == "" {
		t.Fatal("registry JWT does not identify its signing key")
	}
	return keyID
}

func (client *apiClient) doJSON(t *testing.T, method, path string, input, output any, expected int) {
	t.Helper()
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, client.baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Origin", client.baseURL)
	}
	response, err := client.client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if response.StatusCode != expected {
		t.Fatalf("%s %s: got %d, want %d", method, path, response.StatusCode, expected)
	}
	if output != nil {
		if err := json.Unmarshal(raw, output); err != nil {
			t.Fatalf("decode %s %s: %v", method, path, err)
		}
	}
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("Docker CLI is required: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if output, err := run(ctx, "", nil, nil, "docker", "info"); err != nil {
		t.Fatalf("Docker daemon is required: %v\n%s", err, bounded(output))
	}
}

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func waitForRecoveryUI(t *testing.T, baseURL string) string {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	tokenPattern := regexp.MustCompile(`const token="([a-f0-9]{64})"`)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/")
		if err == nil {
			raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				match := tokenPattern.FindSubmatch(raw)
				if len(match) == 2 {
					return string(match[1])
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("recovery UI did not become ready")
	return ""
}

func recoveryUpload(t *testing.T, baseURL, token string, bundle []byte, expectedStatus int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/upload", bytes.NewReader(bundle))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-tar")
	request.Header.Set("X-Grom-Recovery-Token", token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("upload recovery bundle: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		t.Fatalf("upload recovery bundle returned HTTP %d, want %d: %s", response.StatusCode, expectedStatus, raw)
	}
}

func recoveryRestore(t *testing.T, baseURL, token, backupID string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"backupId": backupID, "confirmation": "RESTORE"})
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/restore", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Grom-Recovery-Token", token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("restore through recovery UI: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		t.Fatalf("restore through recovery UI returned HTTP %d: %s", response.StatusCode, body)
	}
}

func waitReady(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/readyz")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("restored installation did not become ready")
}

func onlyBackupSet(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "grom-backup-") {
			return entry.Name()
		}
	}
	t.Fatal("completed backup set was not created")
	return ""
}

func mustRun(t *testing.T, directory string, env []string, stdin io.Reader, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	if output, err := run(ctx, directory, env, stdin, name, args...); err != nil {
		t.Fatalf("%s failed: %v\n%s", name, err, bounded(output))
	}
}

func run(ctx context.Context, directory string, env []string, stdin io.Reader, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir, command.Env, command.Stdin = directory, env, stdin
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	return output.String(), err
}

func withEnv(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

func copyDirectory(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, raw, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func bounded(value string) string {
	if len(value) > 16<<10 {
		return value[len(value)-(16<<10):]
	}
	return value
}
