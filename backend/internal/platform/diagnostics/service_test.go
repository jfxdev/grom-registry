package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/platform/backup"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	"github.com/jfxdev/grom/backend/internal/platform/registrymaintenance"
)

type testDatabase struct{ state database.Diagnostics }

func (probe testDatabase) Inspect(context.Context) database.Diagnostics { return probe.state }

type testDistribution struct {
	version string
	err     error
	wait    bool
}

func (probe testDistribution) Probe(ctx context.Context) (string, error) {
	if probe.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return probe.version, probe.err
}

type testSigning struct {
	algorithm string
	keyID     string
	err       error
}

func (probe testSigning) Probe() (string, string, error) {
	return probe.algorithm, probe.keyID, probe.err
}

type testStorage struct {
	storage registrymaintenance.Storage
	err     error
}

func (probe testStorage) Storage(context.Context) (registrymaintenance.Storage, error) {
	return probe.storage, probe.err
}

type testBackup struct {
	latest *backup.Summary
	err    error
}

func (probe testBackup) Latest(context.Context) (*backup.Summary, error) {
	return probe.latest, probe.err
}

func TestCheckReportsLiveDiagnosticsWithoutSensitiveErrors(t *testing.T) {
	appliedAt := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	checkedAt := appliedAt.Add(time.Minute)
	service := New(Options{
		DatabaseKind: "sqlite",
		Deployment:   Deployment{Profile: "strict"},
		Database: testDatabase{state: database.Diagnostics{
			Kind: database.SQLite, Available: true,
			Migration: database.MigrationDiagnostics{Status: database.MigrationCurrent, AppliedVersion: "202608300001", AppliedAt: &appliedAt},
		}},
		Distribution: testDistribution{version: "registry/2.0"},
		Signing:      testSigning{algorithm: "RS256", keyID: "grom-default"},
		Storage:      testStorage{storage: registrymaintenance.Storage{UsedBytes: 42}},
		Backup:       testBackup{latest: &backup.Summary{CreatedAt: "2026-09-07T11:00:00Z"}},
		Now:          func() time.Time { return checkedAt },
	})

	result := service.Check(context.Background())

	if !result.CheckedAt.Equal(checkedAt) || result.Database.Status != "available" || result.Database.Kind != "sqlite" {
		t.Fatalf("unexpected database result: %#v", result)
	}
	if result.Migration.Status != database.MigrationCurrent || result.Migration.AppliedVersion == nil || *result.Migration.AppliedVersion != "202608300001" || result.Migration.AppliedAt == nil || !result.Migration.AppliedAt.Equal(appliedAt) {
		t.Fatalf("unexpected migration result: %#v", result.Migration)
	}
	if result.Distribution.Status != "available" || result.Distribution.APIVersion == nil || *result.Distribution.APIVersion != "registry/2.0" {
		t.Fatalf("unexpected distribution result: %#v", result.Distribution)
	}
	if result.Signing.Status != "loaded" || result.Signing.Algorithm == nil || *result.Signing.Algorithm != "RS256" || result.Signing.KeyID == nil || *result.Signing.KeyID != "grom-default" {
		t.Fatalf("unexpected signing result: %#v", result.Signing)
	}
	if result.Storage.Status != "available" || result.Storage.UsedBytes == nil || *result.Storage.UsedBytes != 42 {
		t.Fatalf("unexpected storage result: %#v", result.Storage)
	}
	if result.Backup.Status != "available" || result.Backup.LastBackupAt == nil || result.Backup.LastBackupAt.Format(time.RFC3339) != "2026-09-07T11:00:00Z" {
		t.Fatalf("unexpected backup result: %#v", result.Backup)
	}
}

func TestCheckPreservesIndependentFailuresAndNoBackupState(t *testing.T) {
	secretError := errors.New("postgres://operator:secret@db.internal/grom")
	service := New(Options{
		DatabaseKind: "postgres",
		Deployment:   Deployment{Profile: "permissive", InsecureHTTP: true},
		Database: testDatabase{state: database.Diagnostics{
			Kind: database.Postgres, Available: true,
			Migration: database.MigrationDiagnostics{Status: database.MigrationPending},
		}},
		Distribution: testDistribution{err: secretError},
		Signing:      testSigning{err: secretError},
		Storage:      testStorage{err: secretError},
		Backup:       testBackup{},
	})

	result := service.Check(context.Background())
	if result.Database.Status != "available" || result.Migration.Status != database.MigrationPending {
		t.Fatalf("database state was not preserved: %#v", result)
	}
	if result.Distribution.Status != "unavailable" || result.Signing.Status != "unavailable" || result.Storage.Status != "unavailable" {
		t.Fatalf("failed probes must be isolated: %#v", result)
	}
	if result.Backup.Status != "available" || result.Backup.LastBackupAt != nil {
		t.Fatalf("expected available backup agent with no recovery point, got %#v", result.Backup)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "db.internal") {
		t.Fatal("diagnostics leaked a dependency error")
	}
}

func TestCheckReturnsPartialResultWhenProbeTimesOut(t *testing.T) {
	service := New(Options{
		DatabaseKind: "sqlite",
		Deployment:   Deployment{Profile: "development"},
		Database: testDatabase{state: database.Diagnostics{
			Kind: database.SQLite, Available: true,
			Migration: database.MigrationDiagnostics{Status: database.MigrationCurrent},
		}},
		Distribution: testDistribution{wait: true},
		Signing:      testSigning{algorithm: "RS256", keyID: "grom-default"},
		Timeout:      5 * time.Millisecond,
	})

	started := time.Now()
	result := service.Check(context.Background())
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("diagnostic timeout was not bounded: %s", elapsed)
	}
	if result.Database.Status != "available" || result.Signing.Status != "loaded" || result.Distribution.Status != "unavailable" {
		t.Fatalf("unexpected timed result: %#v", result)
	}
}
