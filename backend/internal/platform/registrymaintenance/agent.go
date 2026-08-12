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
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const gromServiceGID = 101

type AgentOptions struct {
	SocketPath string
	ConfigPath string
	DataPath   string
	ReadyURL   string
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
	runtime distributionRuntime
}

type distributionRuntime interface {
	Start(context.Context) error
	Stop(context.Context) error
	Fatal() <-chan error
}

type commandRuntime struct {
	configPath string
	readyURL   string

	mu       sync.Mutex
	command  *exec.Cmd
	done     chan error
	stopping bool
	fatal    chan error
}

var runGarbageCollect = func(ctx context.Context, configPath string) error {
	command := exec.CommandContext(ctx, "registry", "garbage-collect", configPath)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

var newRegistryCommand = func(configPath string) *exec.Cmd {
	command := exec.Command("registry", "serve", configPath)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command
}

var newReadinessClient = func() *http.Client {
	return &http.Client{Timeout: 2 * time.Second}
}

var newDistributionRuntime = func(options AgentOptions) distributionRuntime {
	return &commandRuntime{
		configPath: options.ConfigPath,
		readyURL:   options.ReadyURL,
		fatal:      make(chan error, 1),
	}
}

func Serve(ctx context.Context, options AgentOptions) error {
	if !filepath.IsAbs(options.SocketPath) || !filepath.IsAbs(options.ConfigPath) || !filepath.IsAbs(options.DataPath) {
		return fmt.Errorf("registry maintenance paths must be absolute")
	}
	if options.ReadyURL == "" {
		options.ReadyURL = "http://127.0.0.1:5000/v2/"
	}
	if err := os.MkdirAll(filepath.Dir(options.SocketPath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(options.SocketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	runtime := newDistributionRuntime(options)
	startCtx, cancelStart := context.WithTimeout(ctx, 30*time.Second)
	err := runtime.Start(startCtx)
	cancelStart()
	if err != nil {
		return fmt.Errorf("start Distribution: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = runtime.Stop(stopCtx)
	}()
	c := &controller{options: options, runtime: runtime}
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
		slog.Info("registry garbage collection requested", "data_path", options.DataPath)
		result, err := c.collect(r.Context())
		if err != nil {
			slog.Error("registry garbage collection failed", "error", err, "data_path", options.DataPath)
			http.Error(w, "garbage collection failed", 500)
			return
		}
		slog.Info("registry garbage collection completed",
			"started_at", result.StartedAt,
			"completed_at", result.CompletedAt,
			"bytes_before", result.BytesBefore,
			"bytes_after", result.BytesAfter,
			"reclaimed_bytes", result.ReclaimedBytes,
		)
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
	case err := <-runtime.Fatal():
		return err
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
		slog.Error("could not measure registry storage before garbage collection", "error", err, "data_path", c.options.DataPath)
		return GarbageCollection{}, err
	}
	started := time.Now().UTC()
	if c.runtime == nil {
		return GarbageCollection{}, fmt.Errorf("distribution supervisor is unavailable")
	}
	restart := func() error {
		startCtx, cancelStart := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelStart()
		return c.runtime.Start(startCtx)
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
	stopErr := c.runtime.Stop(stopCtx)
	cancelStop()
	if stopErr != nil {
		if restartErr := restart(); restartErr != nil {
			return GarbageCollection{}, fmt.Errorf("stop Distribution before garbage collection: %v; restart Distribution: %w", stopErr, restartErr)
		}
		return GarbageCollection{}, fmt.Errorf("stop Distribution before garbage collection: %w", stopErr)
	}
	slog.Info("starting Distribution garbage collection", "config_path", c.options.ConfigPath, "bytes_before", before)
	if err := runGarbageCollect(ctx, c.options.ConfigPath); err != nil {
		restartErr := restart()
		if restartErr != nil {
			return GarbageCollection{}, fmt.Errorf("run Distribution garbage collection: %v; restart Distribution: %w", err, restartErr)
		}
		return GarbageCollection{}, fmt.Errorf("run Distribution garbage collection: %w", err)
	}
	after, err := directoryBytes(c.options.DataPath)
	if err != nil {
		restartErr := restart()
		if restartErr != nil {
			return GarbageCollection{}, fmt.Errorf("measure registry storage after garbage collection: %v; restart Distribution: %w", err, restartErr)
		}
		slog.Error("could not measure registry storage after garbage collection", "error", err, "data_path", c.options.DataPath)
		return GarbageCollection{}, err
	}
	if err := restart(); err != nil {
		return GarbageCollection{}, fmt.Errorf("restart Distribution after garbage collection: %w", err)
	}
	result := GarbageCollection{StartedAt: started, CompletedAt: time.Now().UTC(), BytesBefore: before, BytesAfter: after}
	if before > after {
		result.ReclaimedBytes = before - after
	}
	return result, nil
}

func (r *commandRuntime) Start(ctx context.Context) error {
	for {
		r.mu.Lock()
		if r.command == nil {
			break
		}
		if !r.stopping {
			r.mu.Unlock()
			return nil
		}
		done := r.done
		r.mu.Unlock()
		select {
		case <-done:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	command := newRegistryCommand(r.configPath)
	done := make(chan error, 1)
	if err := command.Start(); err != nil {
		r.mu.Unlock()
		return err
	}
	r.command = command
	r.done = done
	r.stopping = false
	r.mu.Unlock()
	go r.wait(command, done)
	if err := r.waitReady(ctx); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.Stop(stopCtx)
		return err
	}
	return nil
}

func (r *commandRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	command, done := r.command, r.done
	if command == nil {
		r.mu.Unlock()
		return nil
	}
	r.stopping = true
	r.mu.Unlock()
	if err := command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *commandRuntime) Fatal() <-chan error { return r.fatal }

func (r *commandRuntime) wait(command *exec.Cmd, done chan error) {
	err := command.Wait()
	r.mu.Lock()
	expected := r.command == command && r.stopping
	if r.command == command {
		r.command = nil
		r.done = nil
		r.stopping = false
	}
	r.mu.Unlock()
	// Publish completion only after clearing the process slot. Stop followed by
	// Start (the GC path) must never mistake the exited child for a live one.
	done <- err
	close(done)
	if !expected {
		if err == nil {
			err = errors.New("process exited without an error")
		}
		select {
		case r.fatal <- fmt.Errorf("distribution exited unexpectedly: %w", err):
		default:
		}
	}
}

func (r *commandRuntime) waitReady(ctx context.Context) error {
	client := newReadinessClient()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.readyURL, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Distribution readiness: %w", ctx.Err())
		case err := <-r.fatal:
			return err
		case <-ticker.C:
		}
	}
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
