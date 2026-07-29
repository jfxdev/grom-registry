package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AgentOptions struct {
	SocketPath         string
	DestinationRoot    string
	GromData           string
	SigningCerts       string
	RegistryData       string
	DistributionConfig string
}

type AgentCreateRequest struct {
	GromVersion       string `json:"gromVersion"`
	DeploymentProfile string `json:"deploymentProfile"`
	Consistency       string `json:"consistency"`
}

func ServeAgent(ctx context.Context, options AgentOptions) error {
	if !filepath.IsAbs(options.SocketPath) {
		return fmt.Errorf("agent socket path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(options.SocketPath), 0o755); err != nil {
		return fmt.Errorf("create agent socket directory: %w", err)
	}
	if info, err := os.Lstat(options.SocketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("agent socket path exists and is not a socket")
		}
		if err := os.Remove(options.SocketPath); err != nil {
			return fmt.Errorf("remove stale agent socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect agent socket: %w", err)
	}
	listener, err := net.Listen("unix", options.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on agent socket: %w", err)
	}
	defer func() { _ = listener.Close() }()
	defer func() { _ = os.Remove(options.SocketPath) }()
	if err := os.Chmod(options.SocketPath, 0o666); err != nil {
		return fmt.Errorf("set agent socket permissions: %w", err)
	}

	var storageMu sync.RWMutex
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/backups", func(w http.ResponseWriter, _ *http.Request) {
		storageMu.RLock()
		defer storageMu.RUnlock()
		summaries, err := List(options.DestinationRoot)
		if err != nil {
			writeAgentError(w, http.StatusInternalServerError)
			return
		}
		writeAgentJSON(w, http.StatusOK, summaries)
	})
	mux.HandleFunc("POST /v1/backups", func(w http.ResponseWriter, r *http.Request) {
		storageMu.Lock()
		defer storageMu.Unlock()
		var input AgentCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Consistency != "quiesced" {
			writeAgentError(w, http.StatusBadRequest)
			return
		}
		inspection, path, err := Create(CreateOptions{
			DestinationRoot: options.DestinationRoot,
			GromData:        options.GromData, SigningCerts: options.SigningCerts,
			RegistryData: options.RegistryData, DistributionConfig: options.DistributionConfig,
			GromVersion: input.GromVersion, DeploymentProfile: input.DeploymentProfile,
			Consistency: input.Consistency,
		})
		if err != nil {
			writeAgentError(w, http.StatusInternalServerError)
			return
		}
		writeAgentJSON(w, http.StatusCreated, Summary{
			BackupID:    inspection.Manifest.BackupID,
			CreatedAt:   inspection.Manifest.CreatedAt.Format(time.RFC3339),
			GromVersion: inspection.Manifest.GromVersion,
			TotalBytes:  inspection.TotalBytes,
			Path:        path,
		})
	})
	mux.HandleFunc("GET /v1/backups/{backupID}/bundle", func(w http.ResponseWriter, r *http.Request) {
		storageMu.RLock()
		defer storageMu.RUnlock()
		backupID := r.PathValue("backupID")
		summary, err := findSummary(options.DestinationRoot, backupID)
		if errors.Is(err, os.ErrNotExist) {
			writeAgentError(w, http.StatusNotFound)
			return
		}
		if err != nil {
			writeAgentError(w, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar"`, filepath.Base(summary.Path)))
		_, err = Bundle(options.DestinationRoot, backupID, w)
		if err != nil {
			return
		}
	})
	mux.HandleFunc("DELETE /v1/backups/{backupID}", func(w http.ResponseWriter, r *http.Request) {
		storageMu.Lock()
		defer storageMu.Unlock()
		_, err := Delete(options.DestinationRoot, r.PathValue("backupID"))
		if errors.Is(err, os.ErrNotExist) {
			writeAgentError(w, http.StatusNotFound)
			return
		}
		if err != nil {
			writeAgentError(w, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errorsCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func writeAgentJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAgentError(w http.ResponseWriter, status int) {
	writeAgentJSON(w, status, map[string]string{"error": http.StatusText(status)})
}
