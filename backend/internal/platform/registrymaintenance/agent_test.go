package registrymaintenance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type readinessRoundTripFunc func(*http.Request) (*http.Response, error)

func (f readinessRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func useReadinessClient(t *testing.T, status int, responseErr error) {
	t.Helper()
	oldClient := newReadinessClient
	t.Cleanup(func() { newReadinessClient = oldClient })
	newReadinessClient = func() *http.Client {
		return &http.Client{Transport: readinessRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if responseErr != nil {
				return nil, responseErr
			}
			return &http.Response{
				StatusCode: status, Status: http.StatusText(status), Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader("ready")), Request: request,
			}, nil
		})}
	}
}

type fakeDistributionRuntime struct {
	starts          int
	stops           int
	fatal           chan error
	startErr        error
	stopErr         error
	stopSawCanceled bool
}

func newFakeDistributionRuntime() *fakeDistributionRuntime {
	return &fakeDistributionRuntime{fatal: make(chan error, 1)}
}

func (r *fakeDistributionRuntime) Start(context.Context) error { r.starts++; return r.startErr }
func (r *fakeDistributionRuntime) Stop(ctx context.Context) error {
	r.stops++
	r.stopSawCanceled = ctx.Err() != nil
	return r.stopErr
}
func (r *fakeDistributionRuntime) Fatal() <-chan error { return r.fatal }

func TestDirectoryBytesCountsOnlyRegularFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one"), make([]byte, 3), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "two"), make([]byte, 5), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "one"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	actual, err := directoryBytes(root)
	if err != nil {
		t.Fatal(err)
	}
	if actual != 8 {
		t.Fatalf("directory bytes = %d, want 8", actual)
	}
}

func TestCollectorReportsStorageAndFailure(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "registry")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "blob"), make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "config.yml")
	if err := os.WriteFile(config, []byte("version: 0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := runGarbageCollect
	t.Cleanup(func() { runGarbageCollect = previous })
	runGarbageCollect = func(ctx context.Context, gotConfig string) error {
		if gotConfig != config {
			t.Fatalf("config = %q", gotConfig)
		}
		return nil
	}
	runtime := newFakeDistributionRuntime()
	gc, err := (&controller{options: AgentOptions{ConfigPath: config, DataPath: data}, runtime: runtime}).collect(context.Background())
	if err != nil || gc.BytesBefore != 7 || gc.BytesAfter != 7 || gc.ReclaimedBytes != 0 {
		t.Fatalf("gc = %#v, %v", gc, err)
	}
	if runtime.stops != 1 || runtime.starts != 1 {
		t.Fatalf("Distribution transitions = stop:%d start:%d, want 1/1", runtime.stops, runtime.starts)
	}
	runGarbageCollect = func(context.Context, string) error { return errors.New("boom") }
	if _, err := (&controller{options: AgentOptions{ConfigPath: config, DataPath: data}, runtime: runtime}).collect(context.Background()); err == nil {
		t.Fatal("expected collection failure")
	}
	if runtime.stops != 2 || runtime.starts != 2 {
		t.Fatalf("failed GC did not restart Distribution: stop:%d start:%d", runtime.stops, runtime.starts)
	}
}

func TestCollectorReportsTransitionFailuresAndReclaimedBytes(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "registry")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	blob := filepath.Join(data, "blob")
	if err := os.WriteFile(blob, make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := runGarbageCollect
	t.Cleanup(func() { runGarbageCollect = previous })

	t.Run("missing supervisor", func(t *testing.T) {
		if _, err := (&controller{options: AgentOptions{DataPath: data}}).collect(context.Background()); err == nil {
			t.Fatal("expected missing supervisor error")
		}
	})

	t.Run("stop and restart", func(t *testing.T) {
		runtime := newFakeDistributionRuntime()
		runtime.stopErr = errors.New("stop failed")
		runtime.startErr = errors.New("restart failed")
		_, err := (&controller{options: AgentOptions{DataPath: data}, runtime: runtime}).collect(context.Background())
		if err == nil || !strings.Contains(err.Error(), "restart Distribution") {
			t.Fatalf("expected combined stop and restart error, got %v", err)
		}
	})

	t.Run("collector and restart", func(t *testing.T) {
		runGarbageCollect = func(context.Context, string) error { return errors.New("collect failed") }
		runtime := newFakeDistributionRuntime()
		runtime.startErr = errors.New("restart failed")
		_, err := (&controller{options: AgentOptions{DataPath: data}, runtime: runtime}).collect(context.Background())
		if err == nil || !strings.Contains(err.Error(), "collect failed") || !strings.Contains(err.Error(), "restart failed") {
			t.Fatalf("expected combined collector and restart error, got %v", err)
		}
	})

	t.Run("measurement and restart", func(t *testing.T) {
		if err := os.WriteFile(blob, make([]byte, 7), 0o600); err != nil {
			t.Fatal(err)
		}
		runGarbageCollect = func(context.Context, string) error { return os.RemoveAll(data) }
		runtime := newFakeDistributionRuntime()
		runtime.startErr = errors.New("restart failed")
		_, err := (&controller{options: AgentOptions{DataPath: data}, runtime: runtime}).collect(context.Background())
		if err == nil || !strings.Contains(err.Error(), "measure registry storage") || !strings.Contains(err.Error(), "restart failed") {
			t.Fatalf("expected combined measurement and restart error, got %v", err)
		}
	})

	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("reclaims bytes", func(t *testing.T) {
		runGarbageCollect = func(context.Context, string) error { return os.WriteFile(blob, []byte("x"), 0o600) }
		result, err := (&controller{options: AgentOptions{DataPath: data}, runtime: newFakeDistributionRuntime()}).collect(context.Background())
		if err != nil || result.ReclaimedBytes != 6 {
			t.Fatalf("collection result = %#v, %v", result, err)
		}
	})

	t.Run("final restart", func(t *testing.T) {
		runGarbageCollect = func(context.Context, string) error { return nil }
		runtime := newFakeDistributionRuntime()
		runtime.startErr = errors.New("restart failed")
		if _, err := (&controller{options: AgentOptions{DataPath: data}, runtime: runtime}).collect(context.Background()); err == nil {
			t.Fatal("expected final restart error")
		}
	})
}

func TestDirectoryBytesRejectsMissingRoot(t *testing.T) {
	if _, err := directoryBytes(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing root error")
	}
}

func TestCollectorUsesOwnedStopContextAndRestartsAfterStopError(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "registry")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeDistributionRuntime()
	runtime.stopErr = errors.New("stop failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&controller{options: AgentOptions{DataPath: data}, runtime: runtime}).collect(ctx)
	if err == nil || runtime.stopSawCanceled {
		t.Fatalf("stop should use an uncanceled agent context: err=%v canceled=%v", err, runtime.stopSawCanceled)
	}
	if runtime.starts != 1 || runtime.stops != 1 {
		t.Fatalf("stop failure recovery transitions = start:%d stop:%d, want 1/1", runtime.starts, runtime.stops)
	}
}

func TestServeRejectsInvalidPathsAndReportsStartupFailure(t *testing.T) {
	if err := Serve(context.Background(), AgentOptions{SocketPath: "agent.sock", ConfigPath: "config.yml", DataPath: "data"}); err == nil {
		t.Fatal("expected relative path rejection")
	}

	root := t.TempDir()
	oldRuntime := newDistributionRuntime
	t.Cleanup(func() { newDistributionRuntime = oldRuntime })
	runtime := newFakeDistributionRuntime()
	runtime.startErr = errors.New("start failed")
	var readyURL string
	newDistributionRuntime = func(options AgentOptions) distributionRuntime {
		readyURL = options.ReadyURL
		return runtime
	}
	err := Serve(context.Background(), AgentOptions{
		SocketPath: filepath.Join(root, "run", "agent.sock"),
		ConfigPath: filepath.Join(root, "config.yml"),
		DataPath:   filepath.Join(root, "data"),
	})
	if err == nil || !strings.Contains(err.Error(), "start Distribution") {
		t.Fatalf("expected startup failure, got %v", err)
	}
	if readyURL != "http://127.0.0.1:5000/v2/" {
		t.Fatalf("default readiness URL = %q", readyURL)
	}
}

func TestCommandRuntimeStartsStopsAndHandlesReadiness(t *testing.T) {
	useReadinessClient(t, http.StatusUnauthorized, nil)
	oldCommand := newRegistryCommand
	t.Cleanup(func() { newRegistryCommand = oldCommand })
	newRegistryCommand = func(string) *exec.Cmd {
		return exec.Command("sh", "-c", "trap 'exit 0' TERM; while :; do sleep 1; done")
	}
	runtime := &commandRuntime{readyURL: "http://distribution.test/v2/", fatal: make(chan error, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("second start should be idempotent: %v", err)
	}
	if err := runtime.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(ctx); err != nil {
		t.Fatalf("stopping an idle runtime should succeed: %v", err)
	}
}

func TestCommandRuntimeReportsStartupAndUnexpectedExit(t *testing.T) {
	oldCommand := newRegistryCommand
	t.Cleanup(func() { newRegistryCommand = oldCommand })

	newRegistryCommand = func(string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing")) }
	runtime := &commandRuntime{readyURL: "http://127.0.0.1:1/v2/", fatal: make(chan error, 1)}
	if err := runtime.Start(context.Background()); err == nil {
		t.Fatal("expected command startup error")
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatalf("startup failure left the runtime locked or published: %v", err)
	}

	newRegistryCommand = func(string) *exec.Cmd { return exec.Command("sh", "-c", "exit 7") }
	useReadinessClient(t, 0, errors.New("not ready"))
	runtime = &commandRuntime{readyURL: "http://127.0.0.1:1/v2/", fatal: make(chan error, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Start(ctx); err == nil || !strings.Contains(err.Error(), "distribution exited unexpectedly") {
		t.Fatalf("expected unexpected-exit error, got %v", err)
	}
}

func TestCommandRuntimeHonorsContextsWhileStoppingAndWaitingForReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := &commandRuntime{
		command:  exec.Command("sh", "-c", "exit 0"),
		done:     make(chan error),
		stopping: true,
		fatal:    make(chan error, 1),
	}
	if err := runtime.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("start while stopping returned %v", err)
	}

	runtime = &commandRuntime{readyURL: unreadableURL("distribution.test"), fatal: make(chan error, 1)}
	if err := runtime.waitReady(context.Background()); err == nil {
		t.Fatal("expected malformed readiness URL error")
	}
	useReadinessClient(t, http.StatusServiceUnavailable, nil)
	runtime.readyURL = "http://distribution.test/v2/"
	if err := runtime.waitReady(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("readiness wait returned %v", err)
	}
}

func unreadableURL(value string) string {
	return ":" + value
}

func TestServeExposesStorageAndCollectorOverUnixSocket(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("the desktop sandbox disallows test Unix sockets")
	}
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "blob"), make([]byte, 4), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "config.yml")
	if err := os.WriteFile(config, []byte("version: 0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := runGarbageCollect
	oldRuntime := newDistributionRuntime
	t.Cleanup(func() { runGarbageCollect = old; newDistributionRuntime = oldRuntime })
	runGarbageCollect = func(context.Context, string) error { return nil }
	fakeRuntime := newFakeDistributionRuntime()
	newDistributionRuntime = func(AgentOptions) distributionRuntime { return fakeRuntime }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	socket := filepath.Join(root, "agent.sock")
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, AgentOptions{SocketPath: socket, ConfigPath: config, DataPath: data}) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket was not created")
		}
		time.Sleep(time.Millisecond)
	}
	client := NewClient(socket)
	storage, err := client.Storage(context.Background())
	if err != nil || storage.UsedBytes != 4 {
		t.Fatalf("storage = %#v, %v", storage, err)
	}
	if _, err := client.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fakeRuntime.starts != 2 || fakeRuntime.stops != 1 {
		t.Fatalf("unexpected supervised transitions: start:%d stop:%d", fakeRuntime.starts, fakeRuntime.stops)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
