package bootacceptance

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/jfxdev/grom/backend/internal/identity/infrastructure/password"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const (
	bootAdminEmail    = "boot-acceptance@grom.local"
	bootAdminPassword = "boot-acceptance-password"
	maxDiagnostic     = 16 << 10
	publicHTTPTimeout = 2 * time.Second
)

// supportedPriorSchema is the reviewed SQLite schema at migration 202607260001,
// the oldest release state supported by this acceptance journey.
//
//go:embed fixtures/sqlite-supported-baseline-202607260001.sql
var supportedPriorSchema string

func TestBootAcceptance(t *testing.T) {
	if os.Getenv("GROM_RUN_BOOT_ACCEPTANCE") != "1" {
		t.Skip("set GROM_RUN_BOOT_ACCEPTANCE=1 or run make test-boot-acceptance")
	}
	requireDocker(t)

	t.Run("empty SQLite installation publishes readiness and API documentation", func(t *testing.T) {
		stack := newStack(t)
		stack.start(t)
		waitReady(t, stack.publicURL)
		login(t, stack.publicURL, bootAdminEmail, bootAdminPassword)
		assertDocumentation(t, stack.publicURL)
		assertRegistryChallenge(t, stack.publicURL)
	})

	t.Run("supported prior SQLite state migrates before readiness", func(t *testing.T) {
		fixture := createSQLiteFixture(t, fixturePreviousVersion)
		stack := newStack(t)
		stack.buildAndSeed(t, fixture)
		stack.startWithoutBuild(t)
		waitReady(t, stack.publicURL)
		login(t, stack.publicURL, "legacy@grom.local", "legacy-password")
		assertProjectVisible(t, stack.publicURL, "legacy")
	})

	t.Run("failed migration never exposes public endpoints or records its version", func(t *testing.T) {
		fixture := createSQLiteFixture(t, fixtureFailingMigration)
		stack := newStack(t)
		stack.buildAndSeed(t, fixture)
		stack.startGromExpectedFailure(t)
		assertNeverPublic(t, stack.publicURL)
		stack.assertMigrationUnapplied(t, "202607270006")
	})
}

type fixtureKind int

const (
	fixturePreviousVersion fixtureKind = iota
	fixtureFailingMigration
)

type stack struct {
	root        string
	composeFile string
	project     string
	publicURL   string
	env         []string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved loopback port: %v", err)
	}
	root := repositoryRoot(t)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	stack := &stack{
		root:        root,
		composeFile: filepath.Join(root, "deploy", "compose", "docker-compose.yml"),
		project:     fmt.Sprintf("grombootacceptance%d%d", os.Getpid(), time.Now().UnixNano()),
		publicURL:   publicURL,
		env: append(os.Environ(),
			"GROM_BIND_ADDRESS=127.0.0.1",
			fmt.Sprintf("GROM_HTTP_PORT=%d", port),
			"GROM_PUBLIC_URL="+publicURL,
			"GROM_DEPLOYMENT_PROFILE=development",
			"GROM_SECURE_COOKIES=false",
			"GROM_BOOTSTRAP_ADMIN_EMAIL="+bootAdminEmail,
			"GROM_BOOTSTRAP_ADMIN_USERNAME=boot-acceptance-admin",
			"GROM_BOOTSTRAP_ADMIN_PASSWORD="+bootAdminPassword,
			"GROM_AUTH_FAILURE_LIMIT=50",
			"GROM_REGISTRY_HTTP_SECRET=boot-acceptance-http-secret",
		),
	}
	t.Cleanup(func() {
		if t.Failed() {
			stack.logDiagnostics(t, "grom")
			stack.logDiagnostics(t, "distribution")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if output, err := stack.compose(ctx, "down", "--volumes", "--remove-orphans"); err != nil {
			t.Errorf("remove isolated Compose project: %v\n%s", err, bounded(output))
		}
	})
	return stack
}

func (s *stack) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	if output, err := s.compose(ctx, "up", "--build", "--detach"); err != nil {
		t.Fatalf("start isolated Compose project: %v\n%s", err, bounded(output))
	}
}

func (s *stack) buildAndSeed(t *testing.T, fixture string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	if output, err := s.compose(ctx, "build"); err != nil {
		t.Fatalf("build isolated Compose image: %v\n%s", err, bounded(output))
	}
	if output, err := s.compose(ctx, "run", "--rm", "--no-deps", "--entrypoint", "sh",
		"-v", filepath.Dir(fixture)+":/fixture:ro", "grom", "-c", "cp /fixture/grom.db /data/grom.db"); err != nil {
		t.Fatalf("seed SQLite fixture: %v\n%s", err, bounded(output))
	}
}

func (s *stack) startWithoutBuild(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if output, err := s.compose(ctx, "up", "--detach", "--no-build"); err != nil {
		t.Fatalf("start seeded Compose project: %v\n%s", err, bounded(output))
	}
}

func (s *stack) startGromExpectedFailure(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := s.compose(ctx, "up", "--detach", "--no-build", "--no-deps", "grom"); err != nil {
		t.Fatalf("start failing-migration Grom container: %v\n%s", err, bounded(output))
	}
}

func (s *stack) assertMigrationUnapplied(t *testing.T, migration string) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "failed.db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := s.compose(ctx, "cp", "grom:/data/grom.db", fixture); err != nil {
		t.Fatalf("copy failed-migration database: %v\n%s", err, bounded(output))
	}
	db, err := sql.Open(sqliteshim.ShimName, fixture+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open failed-migration database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM bun_migrations WHERE name = ?`, migration).Scan(&count); err != nil {
		t.Fatalf("query migration history: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed migration %s was recorded as applied", migration)
	}
}

func (s *stack) compose(ctx context.Context, args ...string) (string, error) {
	commandArgs := []string{"compose", "--ansi", "never", "--project-name", s.project, "--file", s.composeFile}
	commandArgs = append(commandArgs, args...)
	return runCommand(ctx, s.root, s.env, "docker", commandArgs...)
}

func (s *stack) logDiagnostics(t *testing.T, service string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := s.compose(ctx, "logs", "--no-color", "--tail", "160", service)
	if err != nil {
		t.Logf("bounded %s diagnostics unavailable: %v", service, err)
		return
	}
	t.Logf("bounded %s diagnostics:\n%s", service, bounded(output))
}

func createSQLiteFixture(t *testing.T, kind fixtureKind) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "grom.db")
	ctx := context.Background()
	db, databaseKind, err := database.Open(ctx, "sqlite://"+path)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	if kind == fixturePreviousVersion {
		for _, statement := range strings.Split(supportedPriorSchema, ";") {
			if statement = strings.TrimSpace(statement); statement == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, statement); err != nil {
				_ = db.Close()
				t.Fatalf("initialize supported prior schema fixture: %v", err)
			}
		}
		administratorID := foundation.NewID()
		passwordHash, err := password.Hash("legacy-password")
		if err != nil {
			_ = db.Close()
			t.Fatalf("hash legacy administrator password: %v", err)
		}
		now := time.Now().UTC()
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, username, password_hash, is_system_admin, created_at) VALUES (?, ?, ?, ?, ?, ?)`, administratorID.String(), "legacy@grom.local", "legacy-admin", passwordHash, true, now); err != nil {
			_ = db.Close()
			t.Fatalf("seed legacy administrator: %v", err)
		}
		projectID := foundation.NewID()
		if _, err := db.ExecContext(ctx, `INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)`, projectID.String(), "legacy", "Legacy", administratorID.String(), now); err != nil {
			_ = db.Close()
			t.Fatalf("seed legacy project: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO project_memberships (project_id, principal_kind, principal_id, role, created_at) VALUES (?, ?, ?, ?, ?)`, projectID.String(), constants.PrincipalUser, administratorID.String(), constants.RoleAdmin, now); err != nil {
			_ = db.Close()
			t.Fatalf("seed legacy administrator membership: %v", err)
		}
	} else {
		if err := database.Migrate(ctx, db, databaseKind, time.Second, slog.Default()); err != nil {
			_ = db.Close()
			t.Fatalf("migrate failing-migration fixture database: %v", err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM bun_migrations WHERE name = '202607270006'`); err != nil {
			_ = db.Close()
			t.Fatalf("remove current migration record: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	return path
}

func waitReady(t *testing.T, publicURL string) {
	t.Helper()
	client := &http.Client{Timeout: publicHTTPTimeout}
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		response, err := client.Get(publicURL + "/readyz")
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		last = statusOf(response, err)
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("public Grom endpoint did not become ready: %s", last)
}

func assertNeverPublic(t *testing.T, publicURL string) {
	t.Helper()
	client := &http.Client{Timeout: publicHTTPTimeout}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, path := range []string{"/readyz", "/api/v1/me", "/v2/"} {
			response, err := client.Get(publicURL + path)
			if err == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
				_ = response.Body.Close()
				t.Fatalf("failed migration exposed %s with HTTP %d", path, response.StatusCode)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func login(t *testing.T, baseURL, email, password string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: publicHTTPTimeout}
	response, err := client.Post(baseURL+"/api/v1/session", "application/json", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("sign in through public API: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("sign in returned HTTP %d", response.StatusCode)
	}
}

func assertDocumentation(t *testing.T, baseURL string) {
	t.Helper()
	for path, expected := range map[string]string{"/api/openapi.yaml": "openapi: 3.0", "/api/docs": "api-reference"} {
		response, err := (&http.Client{Timeout: publicHTTPTimeout}).Get(baseURL + path)
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), expected) {
			t.Fatalf("unexpected %s response: status=%d", path, response.StatusCode)
		}
	}
}

func assertRegistryChallenge(t *testing.T, baseURL string) {
	t.Helper()
	response, err := (&http.Client{Timeout: publicHTTPTimeout}).Get(baseURL + "/v2/")
	if err != nil {
		t.Fatalf("request registry challenge: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("registry challenge returned HTTP %d", response.StatusCode)
	}
}

func assertProjectVisible(t *testing.T, baseURL, slug string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/session", strings.NewReader(`{"email":"legacy@grom.local","password":"legacy-password"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Jar: mustCookieJar(t), Timeout: publicHTTPTimeout}
	loginResponse, err := client.Do(request)
	if err != nil {
		t.Fatalf("create project-query session: %v", err)
	}
	_ = loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("create project-query session returned HTTP %d", loginResponse.StatusCode)
	}
	projects, err := client.Get(baseURL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("list preserved projects with session: %v", err)
	}
	defer func() { _ = projects.Body.Close() }()
	if projects.StatusCode != http.StatusOK {
		t.Fatalf("list preserved projects returned HTTP %d", projects.StatusCode)
	}
	var body []struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(projects.Body).Decode(&body); err != nil {
		t.Fatalf("decode preserved projects: %v", err)
	}
	for _, project := range body {
		if project.Slug == slug {
			members, err := client.Get(baseURL + "/api/v1/projects/" + slug + "/members")
			if err != nil {
				t.Fatalf("list preserved memberships: %v", err)
			}
			defer func() { _ = members.Body.Close() }()
			if members.StatusCode != http.StatusOK {
				t.Fatalf("list preserved memberships returned HTTP %d", members.StatusCode)
			}
			var memberships []struct {
				PrincipalKind string `json:"principalKind"`
				Role          string `json:"role"`
			}
			if err := json.NewDecoder(members.Body).Decode(&memberships); err != nil {
				t.Fatalf("decode preserved memberships: %v", err)
			}
			for _, membership := range memberships {
				if membership.PrincipalKind == constants.PrincipalUser && membership.Role == constants.RoleAdmin {
					return
				}
			}
			t.Fatalf("preserved administrator membership was not returned")
		}
	}
	t.Fatalf("preserved project %q was not returned", slug)
}

func mustCookieJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("boot acceptance requires the Docker CLI: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if output, err := runCommand(ctx, "", nil, "docker", "info"); err != nil {
		t.Fatalf("boot acceptance requires an accessible Docker daemon: %v\n%s", err, bounded(output))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate boot acceptance package")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", ".."))
}

func runCommand(ctx context.Context, directory string, env []string, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	if env != nil {
		command.Env = env
	}
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	return output.String(), err
}

type limitedBuffer struct{ data []byte }

func (b *limitedBuffer) Write(value []byte) (int, error) {
	b.data = append(b.data, value...)
	if len(b.data) > maxDiagnostic {
		b.data = append([]byte(nil), b.data[len(b.data)-maxDiagnostic:]...)
	}
	return len(value), nil
}

func (b *limitedBuffer) String() string { return string(b.data) }

func bounded(value string) string {
	if len(value) <= maxDiagnostic {
		return value
	}
	return value[:maxDiagnostic] + "\n[diagnostic output truncated]"
}

func statusOf(response *http.Response, err error) string {
	if err != nil {
		return err.Error()
	}
	if response == nil {
		return "no response"
	}
	return response.Status
}
