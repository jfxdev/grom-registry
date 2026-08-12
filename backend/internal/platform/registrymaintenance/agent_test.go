package registrymaintenance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type fakeDistributionRuntime struct {
	starts          int
	stops           int
	fatal           chan error
	stopErr         error
	stopSawCanceled bool
}

func newFakeDistributionRuntime() *fakeDistributionRuntime {
	return &fakeDistributionRuntime{fatal: make(chan error, 1)}
}

func (r *fakeDistributionRuntime) Start(context.Context) error { r.starts++; return nil }
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
