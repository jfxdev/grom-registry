package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	auditstore "github.com/jfxdev/grom/backend/internal/audit/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/jfxdev/grom/backend/internal/httpapi"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitystore "github.com/jfxdev/grom/backend/internal/identity/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/platform/backup"
	"github.com/jfxdev/grom/backend/internal/platform/config"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectstore "github.com/jfxdev/grom/backend/internal/projects/infrastructure/persistence/bun"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/distribution"
	registrystore "github.com/jfxdev/grom/backend/internal/registry/infrastructure/persistence/bun"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("grom stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.InsecureHTTP {
		logger.Warn(
			"grom is using explicitly permitted insecure private HTTP",
			"deployment_profile", cfg.DeploymentProfile,
			"public_url", cfg.PublicURL,
		)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, databaseKind, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()
	if err := database.Migrate(ctx, db, databaseKind, cfg.MigrationLockWait, logger); err != nil {
		return err
	}

	identityRepository := identitystore.New(db)
	projectRepository := projectstore.New(db)
	identityService := identityapp.New(identityRepository, cfg.SessionTTL)
	projectService := projectapp.New(projectRepository)
	registryRepository := registrystore.New(db)
	repositoryService := registryapp.NewRepositoryService(registryRepository)
	projectService.SetDeletionGuard(repositoryService)
	auditService := auditapp.New(auditstore.New(db))
	repositoryService.SetAuditRecorder(auditService)
	inventoryService := registryapp.NewInventoryService(registryRepository)
	maintenanceController := maintenance.New()
	var backupManager *backup.Manager
	if databaseKind == database.SQLite && cfg.BackupAgentSocket != "" {
		backupManager = backup.NewManager(
			backup.NewAgentClient(cfg.BackupAgentSocket),
			maintenanceController,
			func(checkpointContext context.Context) error {
				return database.Checkpoint(checkpointContext, db, databaseKind)
			},
			version,
			string(cfg.DeploymentProfile),
			func(completeContext context.Context, summary backup.Summary) error {
				return auditService.Record(
					completeContext,
					foundation.PrincipalRef{Kind: "system", ID: foundation.ID("grom")},
					constants.AuditBackupCreated,
					constants.AuditResourceBackup,
					foundation.ID(summary.BackupID),
					map[string]any{
						"gromVersion": summary.GromVersion,
						"totalBytes":  summary.TotalBytes,
					},
				)
			},
		)
	}

	restoreMarker, restoreMarkerPath, err := backup.ReadRestoreMarker(cfg.DataDir)
	if err != nil {
		return err
	}
	if restoreMarker.BackupID != "" {
		if err := identityService.InvalidateEphemeralCredentials(ctx); err != nil {
			return fmt.Errorf("invalidate restored ephemeral credentials: %w", err)
		}
		restoreID := foundation.ID(restoreMarker.BackupID)
		if err := auditService.RecordOnce(
			ctx,
			restoreID,
			foundation.PrincipalRef{Kind: "system", ID: foundation.ID("grom")},
			constants.AuditRestoreCompleted,
			constants.AuditResourceBackup,
			restoreID,
			map[string]any{"sourceVersion": restoreMarker.SourceVersion},
		); err != nil {
			return fmt.Errorf("record restore completion: %w", err)
		}
		if err := backup.ConsumeRestoreMarker(restoreMarkerPath); err != nil {
			return fmt.Errorf("consume restore marker: %w", err)
		}
		logger.Info("restored ephemeral credentials invalidated", "backup_id", restoreMarker.BackupID)
	}

	if err := identityService.BootstrapAdmin(
		ctx, cfg.BootstrapEmail, cfg.BootstrapUsername, cfg.BootstrapPassword,
	); err != nil {
		return err
	}

	signer, err := signing.LoadOrCreate(cfg.SigningKeyPath, cfg.SigningCertPath)
	if err != nil {
		return err
	}
	if err := signer.WriteJWKS(cfg.SigningJWKSPath); err != nil {
		return err
	}
	registryTokens := registryapp.NewTokenService(projectService, repositoryService, signer, cfg.RegistryTokenTTL)
	distributionClient, err := distribution.NewClient(cfg.RegistryURL, registryTokens)
	if err != nil {
		return err
	}
	repositoryService.SetExistingRepositoryProbe(distributionClient.RepositoryExists)
	inventoryService.SetDistribution(distributionClient)
	inventoryService.SetProjectResolver(func(ctx context.Context, slug string) (foundation.ID, error) {
		project, err := projectService.Find(ctx, slug)
		if err != nil {
			return "", err
		}
		return project.ID, nil
	})
	lifecycleService := registryapp.NewLifecycleService(
		registryRepository, inventoryService, distributionClient, auditService,
	)
	artifactDeletionService := registryapp.NewArtifactDeletionService(
		registryRepository, repositoryService, inventoryService, distributionClient, auditService,
	)
	registryURL, err := url.Parse(cfg.RegistryURL)
	if err != nil {
		return err
	}
	apiServer, err := httpapi.New(
		identityService, auditService, projectService, repositoryService, inventoryService,
		artifactDeletionService, lifecycleService,
		registryTokens, distributionClient,
		distribution.NewGateway(registryURL, registryTokens, distributionClient, inventoryService.ObservePush, logger),
		logger, cfg.PublicURL, cfg.SecureCookies, cfg.EnableAPIDocs,
		string(cfg.DeploymentProfile), cfg.InsecureHTTP,
		httpapi.SecurityOptions{
			TrustedProxies: cfg.TrustedProxies, AuthFailureLimit: cfg.AuthFailureLimit,
			AuthFailureWindow: cfg.AuthFailureWindow, AuthBlockDuration: cfg.AuthBlockDuration,
		},
		httpapi.OperationalOptions{Backups: backupManager, Maintenance: maintenanceController},
	)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr: cfg.HTTPAddress, Handler: apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info(
			"grom is ready",
			"address", cfg.HTTPAddress,
			"database", databaseKind,
			"deployment_profile", cfg.DeploymentProfile,
			"insecure_http", cfg.InsecureHTTP,
		)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
