package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const RestoreMarkerName = ".grom-restore.json"

type RestoreOptions struct {
	BackupPath               string
	GromDataTarget           string
	SigningCertsTarget       string
	RegistryDataTarget       string
	DistributionConfigTarget string
	TargetGromVersion        string
	PostgresDatabaseURL      string
	Now                      func() time.Time
}

type RestoreMarker struct {
	BackupID      string    `json:"backupId"`
	SourceVersion string    `json:"sourceVersion"`
	RestoredAt    time.Time `json:"restoredAt"`
}

func Restore(options RestoreOptions) (Inspection, error) {
	inspection, err := Inspect(options.BackupPath)
	if err != nil {
		return Inspection{}, err
	}
	if options.TargetGromVersion == "" || inspection.Manifest.GromVersion != options.TargetGromVersion {
		return Inspection{}, fmt.Errorf("backup version is not compatible with the target release")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if inspection.Manifest.Database == "postgres" && options.PostgresDatabaseURL == "" {
		return Inspection{}, fmt.Errorf("PostgreSQL restore requires a database URL")
	}
	targets := []string{options.GromDataTarget, options.SigningCertsTarget, options.RegistryDataTarget}
	for _, target := range targets {
		if !filepath.IsAbs(target) {
			return Inspection{}, fmt.Errorf("restore target paths must be absolute")
		}
		if err := requireEmptyDirectory(target); err != nil {
			return Inspection{}, err
		}
	}
	if !filepath.IsAbs(options.DistributionConfigTarget) {
		return Inspection{}, fmt.Errorf("distribution configuration target must be absolute")
	}
	if _, err := os.Lstat(options.DistributionConfigTarget); !os.IsNotExist(err) {
		return Inspection{}, fmt.Errorf("distribution configuration target must not exist")
	}

	staging := make([]string, len(targets))
	for index, target := range targets {
		staging[index] = filepath.Join(target, ".grom-restore-staging")
		if err := os.Mkdir(staging[index], 0o700); err != nil {
			return Inspection{}, fmt.Errorf("create restore staging: %w", err)
		}
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			for _, path := range staging {
				_ = os.RemoveAll(path)
			}
		}
	}()
	for index := range targets {
		componentIndex := index
		if inspection.Manifest.Database == "postgres" && index > 0 {
			componentIndex++
		}
		if err := extractArchive(
			filepath.Join(options.BackupPath, inspection.Manifest.Components[componentIndex].File),
			staging[index], inspection.Manifest.Components[componentIndex],
		); err != nil {
			return Inspection{}, err
		}
	}
	marker := RestoreMarker{
		BackupID: inspection.Manifest.BackupID, SourceVersion: inspection.Manifest.GromVersion,
		RestoredAt: options.Now().UTC().Truncate(time.Second),
	}
	markerRaw, err := json.Marshal(marker)
	if err != nil {
		return Inspection{}, fmt.Errorf("encode restore marker: %w", err)
	}
	if err := writeSyncedFile(filepath.Join(staging[0], RestoreMarkerName), append(markerRaw, '\n')); err != nil {
		return Inspection{}, err
	}
	if err := os.Chmod(filepath.Join(staging[0], RestoreMarkerName), 0o644); err != nil {
		return Inspection{}, fmt.Errorf("set restore marker permissions: %w", err)
	}
	configRaw, err := os.ReadFile(filepath.Join(options.BackupPath, "distribution-config.yml"))
	if err != nil {
		return Inspection{}, fmt.Errorf("read restored distribution configuration: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(options.DistributionConfigTarget), 0o700); err != nil {
		return Inspection{}, fmt.Errorf("create distribution configuration target directory: %w", err)
	}
	configStaging := options.DistributionConfigTarget + ".grom-restore-staging"
	if _, err := os.Lstat(configStaging); !os.IsNotExist(err) {
		return Inspection{}, fmt.Errorf("distribution configuration staging target already exists")
	}
	if err := writeSyncedFile(configStaging, configRaw); err != nil {
		return Inspection{}, err
	}
	defer func() {
		if cleanupStaging {
			_ = os.Remove(configStaging)
		}
	}()
	if inspection.Manifest.Database == "sqlite" {
		if info, err := os.Stat(filepath.Join(staging[0], "grom.db")); err != nil || !info.Mode().IsRegular() {
			return Inspection{}, fmt.Errorf("restored Grom data does not contain grom.db")
		}
	}
	for _, name := range []string{"signing-key.pem", "signing-cert.pem", "jwks.json"} {
		if info, err := os.Stat(filepath.Join(staging[1], name)); err != nil || !info.Mode().IsRegular() {
			return Inspection{}, fmt.Errorf("restored signing material is incomplete")
		}
	}
	for index, target := range targets {
		if err := applyDirectoryMetadata(staging[index], target); err != nil {
			return Inspection{}, err
		}
	}
	type promotedEntry struct {
		staged string
		target string
	}
	promoted := make([]promotedEntry, 0)
	configPromoted := false
	rollbackPromotion := func(cause error) error {
		var rollbackFailures []error
		if configPromoted {
			if err := os.Rename(options.DistributionConfigTarget, configStaging); err != nil {
				rollbackFailures = append(rollbackFailures, err)
			}
		}
		for index := len(promoted) - 1; index >= 0; index-- {
			move := promoted[index]
			if err := os.MkdirAll(filepath.Dir(move.staged), 0o700); err != nil {
				rollbackFailures = append(rollbackFailures, err)
				continue
			}
			if err := os.Rename(move.target, move.staged); err != nil {
				rollbackFailures = append(rollbackFailures, err)
			}
		}
		if len(rollbackFailures) != 0 {
			cleanupStaging = false
			return fmt.Errorf("%w; restore rollback failed and restore targets are dirty; wipe them before retrying: %v", cause, rollbackFailures)
		}
		return cause
	}
	for index, target := range targets {
		entries, err := os.ReadDir(staging[index])
		if err != nil {
			return Inspection{}, rollbackPromotion(fmt.Errorf("list restore staging: %w", err))
		}
		for _, entry := range entries {
			stagedPath := filepath.Join(staging[index], entry.Name())
			targetPath := filepath.Join(target, entry.Name())
			if err := os.Rename(stagedPath, targetPath); err != nil {
				return Inspection{}, rollbackPromotion(fmt.Errorf("promote restored component: %w", err))
			}
			promoted = append(promoted, promotedEntry{staged: stagedPath, target: targetPath})
		}
	}
	if err := os.Rename(configStaging, options.DistributionConfigTarget); err != nil {
		return Inspection{}, rollbackPromotion(fmt.Errorf("promote restored distribution configuration: %w", err))
	}
	configPromoted = true
	if inspection.Manifest.Database == "postgres" {
		if err := restorePostgres(options.PostgresDatabaseURL, filepath.Join(options.BackupPath, "postgres.dump")); err != nil {
			return Inspection{}, rollbackPromotion(fmt.Errorf("restore postgres database: %w", err))
		}
	}
	for _, path := range staging {
		if err := os.Remove(path); err != nil {
			return Inspection{}, rollbackPromotion(fmt.Errorf("remove restore staging: %w", err))
		}
	}
	cleanupStaging = false
	return inspection, nil
}

func restorePostgres(databaseURL, dumpPath string) error {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("parse postgres target URL: %w", err)
	}
	password, _ := u.User.Password()
	u.User = url.User(u.User.Username())
	command := exec.Command("pg_restore", "--single-transaction", "--clean", "--if-exists", "--no-owner", "--no-privileges", "--dbname="+u.String(), dumpPath)
	command.Env = append(os.Environ(), "PGPASSWORD="+password)
	if _, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}
	return nil
}

func requireEmptyDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("restore target must be an existing directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("inspect restore target: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("restore target must be empty")
	}
	return nil
}

func ReadRestoreMarker(dataDir string) (RestoreMarker, string, error) {
	path := filepath.Join(dataDir, RestoreMarkerName)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return RestoreMarker{}, path, nil
	}
	if err != nil {
		return RestoreMarker{}, path, fmt.Errorf("read restore marker: %w", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, 64<<10))
	if err != nil {
		return RestoreMarker{}, path, fmt.Errorf("read restore marker: %w", err)
	}
	var marker RestoreMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		return RestoreMarker{}, path, fmt.Errorf("decode restore marker: %w", err)
	}
	if _, err := uuid.Parse(marker.BackupID); err != nil || marker.SourceVersion == "" || marker.RestoredAt.IsZero() {
		return RestoreMarker{}, path, fmt.Errorf("restore marker is invalid")
	}
	return marker, path, nil
}

func ConsumeRestoreMarker(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove restore marker: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
}
