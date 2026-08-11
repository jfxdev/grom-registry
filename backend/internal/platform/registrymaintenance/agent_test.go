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
	gc, err := (&controller{options: AgentOptions{ConfigPath: config, DataPath: data}}).collect(context.Background())
	if err != nil || gc.BytesBefore != 7 || gc.BytesAfter != 7 || gc.ReclaimedBytes != 0 {
		t.Fatalf("gc = %#v, %v", gc, err)
	}
	runGarbageCollect = func(context.Context, string) error { return errors.New("boom") }
	if _, err := (&controller{options: AgentOptions{ConfigPath: config, DataPath: data}}).collect(context.Background()); err == nil {
		t.Fatal("expected collection failure")
	}
}

func TestDirectoryBytesRejectsMissingRoot(t *testing.T) {
	if _, err := directoryBytes(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing root error")
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
	t.Cleanup(func() { runGarbageCollect = old })
	runGarbageCollect = func(context.Context, string) error { return nil }
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
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
