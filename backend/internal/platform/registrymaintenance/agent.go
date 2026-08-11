// Package registrymaintenance owns the small, local control surface used to
// place Distribution in read-only mode and run its stop-the-world collector.
// It deliberately exposes only a Unix socket: Grom never receives Docker
// socket access or direct write access to the registry volume.
package registrymaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const gromServiceGID = 101

type AgentOptions struct {
	SocketPath string
	ConfigPath string
	DataPath   string
}

type Storage struct {
	UsedBytes int64 `json:"usedBytes"`
}

type GarbageCollection struct {
	StartedAt      time.Time `json:"startedAt"`
	CompletedAt    time.Time `json:"completedAt"`
	BytesBefore    int64     `json:"bytesBefore"`
	BytesAfter     int64     `json:"bytesAfter"`
	ReclaimedBytes int64     `json:"reclaimedBytes"`
}

type controller struct {
	options AgentOptions
	mu      sync.Mutex
}

var runGarbageCollect = func(ctx context.Context, configPath string) error {
	command := exec.CommandContext(ctx, "registry", "garbage-collect", configPath)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func Serve(ctx context.Context, options AgentOptions) error {
	if !filepath.IsAbs(options.SocketPath) || !filepath.IsAbs(options.ConfigPath) || !filepath.IsAbs(options.DataPath) {
		return fmt.Errorf("registry maintenance paths must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(options.SocketPath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(options.SocketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	c := &controller{options: options}
	listener, err := net.Listen("unix", options.SocketPath)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close(); _ = os.Remove(options.SocketPath) }()
	if os.Geteuid() == 0 {
		if err := os.Chown(options.SocketPath, -1, gromServiceGID); err != nil {
			return err
		}
	}
	if err := os.Chmod(options.SocketPath, 0o660); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/storage", func(w http.ResponseWriter, _ *http.Request) {
		used, err := directoryBytes(options.DataPath)
		if err != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		writeJSON(w, 200, Storage{UsedBytes: used})
	})
	mux.HandleFunc("POST /v1/garbage-collections", func(w http.ResponseWriter, r *http.Request) {
		result, err := c.collect(r.Context())
		if err != nil {
			http.Error(w, "garbage collection failed", 500)
			return
		}
		writeJSON(w, 200, result)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (c *controller) collect(ctx context.Context) (GarbageCollection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	before, err := directoryBytes(c.options.DataPath)
	if err != nil {
		return GarbageCollection{}, err
	}
	started := time.Now().UTC()
	if err := runGarbageCollect(ctx, c.options.ConfigPath); err != nil {
		return GarbageCollection{}, fmt.Errorf("run distribution garbage collection: %w", err)
	}
	after, err := directoryBytes(c.options.DataPath)
	if err != nil {
		return GarbageCollection{}, err
	}
	result := GarbageCollection{StartedAt: started, CompletedAt: time.Now().UTC(), BytesBefore: before, BytesAfter: after}
	if before > after {
		result.ReclaimedBytes = before - after
	}
	return result, nil
}

func directoryBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
