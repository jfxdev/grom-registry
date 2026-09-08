// Package diagnostics aggregates safe, live installation state for the
// administrator-facing control plane. It deliberately reports each dependency
// independently and never returns configuration or credential material.
package diagnostics

import (
	"context"
	"time"

	"github.com/jfxdev/grom/backend/internal/platform/backup"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/jfxdev/grom/backend/internal/platform/registrymaintenance"
)

const CheckTimeout = 5 * time.Second

type Deployment struct {
	Profile      string `json:"profile"`
	InsecureHTTP bool   `json:"insecureHttp"`
}

type Result struct {
	CheckedAt    time.Time    `json:"checkedAt"`
	Deployment   Deployment   `json:"deployment"`
	Database     Database     `json:"database"`
	Migration    Migration    `json:"migration"`
	Signing      Signing      `json:"signing"`
	Distribution Distribution `json:"distribution"`
	Storage      Storage      `json:"storage"`
	Backup       Backup       `json:"backup"`
}

type Database struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type Migration struct {
	Status         string     `json:"status"`
	AppliedVersion *string    `json:"appliedVersion"`
	AppliedAt      *time.Time `json:"appliedAt"`
}

type Signing struct {
	Status    string  `json:"status"`
	Algorithm *string `json:"algorithm"`
	KeyID     *string `json:"keyId"`
}

type Distribution struct {
	Status     string  `json:"status"`
	APIVersion *string `json:"apiVersion"`
}

type Storage struct {
	Status    string `json:"status"`
	UsedBytes *int64 `json:"usedBytes"`
}

type Backup struct {
	Status       string     `json:"status"`
	LastBackupAt *time.Time `json:"lastBackupAt"`
}

type DatabaseProber interface {
	Inspect(context.Context) database.Diagnostics
}

type DistributionProber interface {
	Probe(context.Context) (string, error)
}

type SigningProber interface {
	Probe() (algorithm, keyID string, err error)
}

type StorageProber interface {
	Storage(context.Context) (registrymaintenance.Storage, error)
}

type BackupProber interface {
	Latest(context.Context) (*backup.Summary, error)
}

type Options struct {
	DatabaseKind string
	Deployment   Deployment
	Database     DatabaseProber
	Distribution DistributionProber
	Signing      SigningProber
	Storage      StorageProber
	Backup       BackupProber
	Now          func() time.Time
	Timeout      time.Duration
}

type Service struct {
	databaseKind string
	deployment   Deployment
	database     DatabaseProber
	distribution DistributionProber
	signing      SigningProber
	storage      StorageProber
	backup       BackupProber
	now          func() time.Time
	timeout      time.Duration
}

func New(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = CheckTimeout
	}
	return &Service{
		databaseKind: options.DatabaseKind,
		deployment:   options.Deployment,
		database:     options.Database,
		distribution: options.Distribution,
		signing:      options.Signing,
		storage:      options.Storage,
		backup:       options.Backup,
		now:          now,
		timeout:      timeout,
	}
}

// Check runs fresh dependency checks in parallel. A failed or timed-out check
// changes only its own status; the result is intentionally useful during a
// partial outage.
func (service *Service) Check(ctx context.Context) Result {
	if service == nil {
		return Result{
			CheckedAt:    time.Now().UTC(),
			Database:     Database{Status: "unavailable"},
			Migration:    Migration{Status: database.MigrationUnavailable},
			Signing:      Signing{Status: "unavailable"},
			Distribution: Distribution{Status: "unavailable"},
			Storage:      Storage{Status: "unavailable"},
			Backup:       Backup{Status: "unavailable"},
		}
	}
	result := Result{
		Deployment:   service.deployment,
		Database:     Database{Kind: service.databaseKind, Status: "unavailable"},
		Migration:    Migration{Status: database.MigrationUnavailable},
		Signing:      Signing{Status: "unavailable"},
		Distribution: Distribution{Status: "unavailable"},
		Storage:      Storage{Status: "unavailable"},
		Backup:       Backup{Status: "unavailable"},
	}
	checkContext, cancel := context.WithTimeout(ctx, service.timeout)
	defer cancel()
	updates := make(chan func(*Result), 5)

	go func() {
		if service.database == nil {
			updates <- func(*Result) {}
			return
		}
		state := service.database.Inspect(checkContext)
		updates <- func(result *Result) {
			result.Database.Kind = string(state.Kind)
			if state.Available {
				result.Database.Status = "available"
			}
			result.Migration.Status = state.Migration.Status
			if state.Migration.AppliedVersion != "" {
				version := state.Migration.AppliedVersion
				result.Migration.AppliedVersion = &version
			}
			result.Migration.AppliedAt = state.Migration.AppliedAt
		}
	}()
	go func() {
		if service.distribution == nil {
			updates <- func(*Result) {}
			return
		}
		version, err := service.distribution.Probe(checkContext)
		updates <- func(result *Result) {
			if err != nil {
				return
			}
			result.Distribution.Status = "available"
			if version != "" {
				result.Distribution.APIVersion = &version
			}
		}
	}()
	go func() {
		if service.signing == nil {
			updates <- func(*Result) {}
			return
		}
		algorithm, keyID, err := service.signing.Probe()
		updates <- func(result *Result) {
			if err != nil {
				return
			}
			result.Signing.Status = "loaded"
			result.Signing.Algorithm = &algorithm
			result.Signing.KeyID = &keyID
		}
	}()
	go func() {
		if service.storage == nil {
			updates <- func(*Result) {}
			return
		}
		storage, err := service.storage.Storage(checkContext)
		updates <- func(result *Result) {
			if err != nil {
				return
			}
			result.Storage.Status = "available"
			usedBytes := storage.UsedBytes
			result.Storage.UsedBytes = &usedBytes
		}
	}()
	go func() {
		if service.backup == nil {
			updates <- func(*Result) {}
			return
		}
		latest, err := service.backup.Latest(checkContext)
		updates <- func(result *Result) {
			if err != nil {
				return
			}
			result.Backup.Status = "available"
			if latest == nil {
				return
			}
			createdAt, parseErr := time.Parse(time.RFC3339, latest.CreatedAt)
			if parseErr == nil {
				createdAt = createdAt.UTC()
				result.Backup.LastBackupAt = &createdAt
			}
		}
	}()

	for completed := 0; completed < cap(updates); completed++ {
		select {
		case update := <-updates:
			update(&result)
		case <-checkContext.Done():
			result.CheckedAt = service.now().UTC()
			return result
		}
	}
	result.CheckedAt = service.now().UTC()
	return result
}
