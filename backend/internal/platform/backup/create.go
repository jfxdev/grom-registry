package backup

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

type CreateOptions struct {
	DestinationRoot    string
	GromData           string
	SigningCerts       string
	RegistryData       string
	DistributionConfig string
	PostgresDump       string
	Database           string
	GromVersion        string
	DeploymentProfile  string
	Consistency        string
	Now                func() time.Time
}

func Create(options CreateOptions) (Inspection, string, error) {
	if !filepath.IsAbs(options.DestinationRoot) {
		return Inspection{}, "", fmt.Errorf("backup destination must be absolute")
	}
	if err := validateManifestMetadata(options.GromVersion, options.DeploymentProfile); err != nil {
		return Inspection{}, "", err
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	for _, source := range []string{options.GromData, options.SigningCerts, options.RegistryData, options.DistributionConfig} {
		if !filepath.IsAbs(source) {
			return Inspection{}, "", fmt.Errorf("backup source paths must be absolute")
		}
	}
	if options.Database == "" {
		options.Database = "sqlite"
	}
	if options.Database != "sqlite" && options.Database != "postgres" {
		return Inspection{}, "", fmt.Errorf("unsupported database profile")
	}
	if options.Database == "postgres" && !filepath.IsAbs(options.PostgresDump) {
		return Inspection{}, "", fmt.Errorf("PostgreSQL dump path must be absolute")
	}
	if err := os.MkdirAll(options.DestinationRoot, 0o700); err != nil {
		return Inspection{}, "", fmt.Errorf("create backup destination: %w", err)
	}
	if err := os.Chmod(options.DestinationRoot, 0o700); err != nil {
		return Inspection{}, "", fmt.Errorf("secure backup destination: %w", err)
	}
	id := uuid.NewString()
	createdAt := options.Now().UTC().Truncate(time.Second)
	name := fmt.Sprintf("grom-backup-%s-%s", createdAt.Format("20060102T150405Z"), id)
	partial := filepath.Join(options.DestinationRoot, ".partial-"+id)
	final := filepath.Join(options.DestinationRoot, name)
	if err := os.Mkdir(partial, 0o700); err != nil {
		return Inspection{}, "", fmt.Errorf("create partial backup set: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(partial)
		}
	}()

	consistency := options.Consistency
	formatVersion := FormatVersion
	if consistency == "" {
		consistency = "offline"
	}
	if consistency == "quiesced" {
		formatVersion = QuiescedFormatVersion
	}
	if options.Database == "postgres" {
		formatVersion = PostgresFormatVersion
	}
	manifest := Manifest{
		FormatVersion: formatVersion, BackupID: id, CreatedAt: createdAt,
		Consistency: consistency, GromVersion: options.GromVersion,
		Database: options.Database, RegistryStorage: "filesystem",
		DeploymentProfile: options.DeploymentProfile,
		Components:        make([]Component, len(requiredComponents)),
	}
	expected := requiredComponents
	if options.Database == "postgres" {
		expected = postgresRequiredComponents
		manifest.Components = make([]Component, len(expected))
	}
	if options.Database == "postgres" {
		component, err := copyRegularFile(options.PostgresDump, filepath.Join(partial, expected[1].File))
		if err != nil {
			return Inspection{}, "", err
		}
		component.Name, component.File, component.Kind = expected[1].Name, expected[1].File, "file"
		manifest.Components[1] = component
		for sourceIndex, source := range []string{options.GromData, options.SigningCerts, options.RegistryData} {
			component, err := writeArchive(source, filepath.Join(partial, expected[sourceIndex+2].File))
			if err != nil {
				return Inspection{}, "", err
			}
			component.Name, component.File, component.Kind = expected[sourceIndex+2].Name, expected[sourceIndex+2].File, "tar"
			manifest.Components[sourceIndex+2] = component
		}
		configComponent, err := copyRegularFile(options.DistributionConfig, filepath.Join(partial, expected[4].File))
		if err != nil {
			return Inspection{}, "", err
		}
		configComponent.Name, configComponent.File, configComponent.Kind = expected[4].Name, expected[4].File, "file"
		manifest.Components[4] = configComponent
	} else {
		sources := []string{options.GromData, options.SigningCerts, options.RegistryData}
		for index, source := range sources {
			component, err := writeArchive(source, filepath.Join(partial, requiredComponents[index].File))
			if err != nil {
				return Inspection{}, "", err
			}
			component.Name, component.File, component.Kind = requiredComponents[index].Name, requiredComponents[index].File, "tar"
			manifest.Components[index] = component
		}
		configComponent, err := copyRegularFile(options.DistributionConfig, filepath.Join(partial, requiredComponents[3].File))
		if err != nil {
			return Inspection{}, "", err
		}
		configComponent.Name, configComponent.File, configComponent.Kind = requiredComponents[3].Name, requiredComponents[3].File, "file"
		manifest.Components[3] = configComponent
	}

	if err := validateManifest(manifest); err != nil {
		return Inspection{}, "", err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Inspection{}, "", fmt.Errorf("encode manifest: %w", err)
	}
	raw = append(raw, '\n')
	if err := writeSyncedFile(filepath.Join(partial, "manifest.json"), raw); err != nil {
		return Inspection{}, "", err
	}
	hashes := map[string]string{"manifest.json": checksumBytes(raw)}
	for _, component := range manifest.Components {
		hashes[component.File] = component.SHA256
	}
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	var checksumContent []byte
	for _, name := range names {
		checksumContent = fmt.Appendf(checksumContent, "%s  %s\n", hashes[name], name)
	}
	if err := writeSyncedFile(filepath.Join(partial, "SHA256SUMS"), checksumContent); err != nil {
		return Inspection{}, "", err
	}
	if err := writeSyncedFile(filepath.Join(partial, "COMPLETE"), []byte{}); err != nil {
		return Inspection{}, "", err
	}
	inspection, err := Inspect(partial)
	if err != nil {
		return Inspection{}, "", fmt.Errorf("validate completed backup: %w", err)
	}
	if err := syncDirectory(partial); err != nil {
		return Inspection{}, "", err
	}
	if err := os.Rename(partial, final); err != nil {
		return Inspection{}, "", fmt.Errorf("publish backup set: %w", err)
	}
	published = true
	if err := syncDirectory(options.DestinationRoot); err != nil {
		return Inspection{}, "", err
	}
	return inspection, final, nil
}

func copyRegularFile(source, destination string) (Component, error) {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return Component{}, fmt.Errorf("distribution configuration must be a regular file")
	}
	if info.Size() > MaxConfigSize {
		return Component{}, fmt.Errorf("distribution configuration exceeds %d bytes", MaxConfigSize)
	}
	input, err := os.Open(source)
	if err != nil {
		return Component{}, fmt.Errorf("open distribution configuration: %w", err)
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Component{}, fmt.Errorf("create distribution configuration backup: %w", err)
	}
	hashWriter := &hashingWriter{writer: output, hash: sha256.New()}
	bytes, copyErr := io.Copy(hashWriter, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		return Component{}, fmt.Errorf("copy distribution configuration")
	}
	return Component{Bytes: bytes, SHA256: hashWriter.sum(), EntryCount: 1, ContentBytes: bytes}, nil
}

type hashingWriter struct {
	writer io.Writer
	hash   hash.Hash
}

func (writer *hashingWriter) Write(value []byte) (int, error) {
	written, err := writer.writer.Write(value)
	if written > 0 {
		_, _ = writer.hash.Write(value[:written])
	}
	return written, err
}

func (writer *hashingWriter) sum() string { return fmt.Sprintf("%x", writer.hash.Sum(nil)) }

func writeSyncedFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create backup metadata: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write backup metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync backup metadata: %w", err)
	}
	return file.Close()
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return fmt.Errorf("sync directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("close synced directory: %w", err)
	}
	return nil
}
