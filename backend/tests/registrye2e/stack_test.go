package registrye2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	e2eAdminEmail    = "registry-e2e@grom.local"
	e2eAdminPassword = "registry-e2e-admin-password"
	e2eTokenTTL      = 8 * time.Second
	maxDiagnostic    = 16 << 10
	cleanupTimeout   = 90 * time.Second
)

type testStack struct {
	root        string
	composeFile string
	project     string
	publicURL   string
	registry    string
	env         []string
}

func startTestStack(t *testing.T) *testStack {
	t.Helper()
	requireDocker(t)

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
	stack := &testStack{
		root:        root,
		composeFile: filepath.Join(root, "deploy", "compose", "docker-compose.yml"),
		project:     fmt.Sprintf("gromregistrye2e%d%d", os.Getpid(), time.Now().UnixNano()),
		publicURL:   publicURL,
		registry:    strings.TrimPrefix(publicURL, "http://"),
		env: append(os.Environ(),
			"GROM_BIND_ADDRESS=127.0.0.1",
			fmt.Sprintf("GROM_HTTP_PORT=%d", port),
			"GROM_PUBLIC_URL="+publicURL,
			"GROM_DEPLOYMENT_PROFILE=development",
			"GROM_SECURE_COOKIES=false",
			"GROM_BOOTSTRAP_ADMIN_EMAIL="+e2eAdminEmail,
			"GROM_BOOTSTRAP_ADMIN_USERNAME=registry-e2e-admin",
			"GROM_BOOTSTRAP_ADMIN_PASSWORD="+e2eAdminPassword,
			"GROM_REGISTRY_TOKEN_TTL="+e2eTokenTTL.String(),
			"GROM_AUTH_FAILURE_LIMIT=50",
			"GROM_AUTH_FAILURE_WINDOW=1m",
			"GROM_AUTH_BLOCK_DURATION=1m",
			"GROM_REGISTRY_HTTP_SECRET=registry-e2e-http-secret",
		),
	}

	// Cleanup is registered before Compose starts so partial startup is covered.
	t.Cleanup(func() {
		if t.Failed() {
			stack.logDiagnostics(t, "grom")
			stack.logDiagnostics(t, "distribution")
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if output, downErr := stack.compose(ctx, "down", "--volumes", "--remove-orphans"); downErr != nil {
			t.Errorf("remove isolated Compose project: %v\n%s", downErr, bounded(output))
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	output, err := stack.compose(ctx, "up", "--build", "--detach")
	if err != nil {
		t.Fatalf("start isolated Compose project: %v\n%s", err, bounded(output))
	}
	waitForPublicStack(t, stack.publicURL)
	return stack
}

func (s *testStack) logDiagnostics(t *testing.T, service string) {
	t.Helper()
	logsCtx, logsCancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer logsCancel()
	output, err := s.compose(logsCtx, "logs", "--no-color", "--tail", "160", service)
	if err != nil {
		t.Logf("bounded %s diagnostics unavailable: %v", service, err)
		return
	}
	t.Logf("bounded %s diagnostics:\n%s", service, bounded(output))
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("registry E2E requires the Docker CLI: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if output, err := runCommand(ctx, "", nil, nil, "docker", "info"); err != nil {
		t.Fatalf("registry E2E requires an accessible Docker daemon: %v\n%s", err, bounded(output))
	}
	if output, err := runCommand(ctx, "", nil, nil, "docker", "compose", "version"); err != nil {
		t.Fatalf("registry E2E requires Docker Compose: %v\n%s", err, bounded(output))
	}
}

func waitForPublicStack(t *testing.T, publicURL string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		ready, readyErr := client.Get(publicURL + "/readyz")
		if readyErr == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(ready.Body, 4<<10))
			_ = ready.Body.Close()
		}
		registry, registryErr := client.Get(publicURL + "/v2/")
		if registryErr == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(registry.Body, 4<<10))
			_ = registry.Body.Close()
		}
		if readyErr == nil && ready.StatusCode == http.StatusOK &&
			registryErr == nil && (registry.StatusCode == http.StatusOK || registry.StatusCode == http.StatusUnauthorized) {
			return
		}
		last = fmt.Sprintf("readiness=%v registry=%v", statusOf(ready, readyErr), statusOf(registry, registryErr))
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("public Grom endpoint did not become ready: %s", last)
}

func statusOf(response *http.Response, err error) any {
	if err != nil {
		return err
	}
	if response == nil {
		return "no response"
	}
	return response.StatusCode
}

func (s *testStack) compose(ctx context.Context, args ...string) (string, error) {
	commandArgs := []string{"compose", "--ansi", "never", "--project-name", s.project, "--file", s.composeFile}
	commandArgs = append(commandArgs, args...)
	return runCommand(ctx, s.root, s.env, nil, "docker", commandArgs...)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate registry E2E package")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", ".."))
}

func runCommand(ctx context.Context, directory string, env []string, stdin io.Reader, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	if env != nil {
		command.Env = env
	}
	command.Stdin = stdin
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	return output.String(), err
}

type limitedBuffer struct {
	data []byte
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	if len(value) >= maxDiagnostic {
		b.data = append(b.data[:0], value[len(value)-maxDiagnostic:]...)
		return len(value), nil
	}
	b.data = append(b.data, value...)
	if len(b.data) > maxDiagnostic {
		copy(b.data, b.data[len(b.data)-maxDiagnostic:])
		b.data = b.data[:maxDiagnostic]
	}
	return len(value), nil
}

func (b *limitedBuffer) String() string {
	return string(b.data)
}

func bounded(value string) string {
	if len(value) <= maxDiagnostic {
		return value
	}
	return value[:maxDiagnostic] + "\n[diagnostic output truncated]"
}
