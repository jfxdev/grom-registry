package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	FormatVersion         = 1
	QuiescedFormatVersion = 2
	MaxManifestSize       = 1 << 20
	MaxArchivePath        = 4096
	MaxConfigSize         = 16 << 20
)

const (
	ComponentGromData           = "grom-data"
	ComponentSigningCerts       = "signing-certs"
	ComponentRegistryData       = "registry-data"
	ComponentDistributionConfig = "distribution-config"
)

type Manifest struct {
	FormatVersion     int         `json:"formatVersion"`
	BackupID          string      `json:"backupId"`
	CreatedAt         time.Time   `json:"createdAt"`
	Consistency       string      `json:"consistency"`
	GromVersion       string      `json:"gromVersion"`
	Database          string      `json:"database"`
	RegistryStorage   string      `json:"registryStorage"`
	DeploymentProfile string      `json:"deploymentProfile"`
	Components        []Component `json:"components"`
}

type Component struct {
	Name         string `json:"name"`
	File         string `json:"file"`
	Kind         string `json:"kind"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	EntryCount   int64  `json:"entryCount"`
	ContentBytes int64  `json:"contentBytes"`
}

var requiredComponents = []Component{
	{Name: ComponentGromData, File: "grom-data.tar", Kind: "tar"},
	{Name: ComponentSigningCerts, File: "signing-certs.tar", Kind: "tar"},
	{Name: ComponentRegistryData, File: "registry-data.tar", Kind: "tar"},
	{Name: ComponentDistributionConfig, File: "distribution-config.yml", Kind: "file"},
}

func validateManifest(manifest Manifest) error {
	if manifest.FormatVersion != FormatVersion && manifest.FormatVersion != QuiescedFormatVersion {
		return fmt.Errorf("unsupported backup format version %d", manifest.FormatVersion)
	}
	if _, err := uuid.Parse(manifest.BackupID); err != nil {
		return fmt.Errorf("backup ID is invalid")
	}
	if manifest.CreatedAt.IsZero() || manifest.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("backup creation time must be UTC")
	}
	if (manifest.FormatVersion == FormatVersion && manifest.Consistency != "offline") ||
		(manifest.FormatVersion == QuiescedFormatVersion && manifest.Consistency != "quiesced") {
		return fmt.Errorf("unsupported consistency mode")
	}
	if manifest.Database != "sqlite" || manifest.RegistryStorage != "filesystem" {
		return fmt.Errorf("unsupported database or registry storage profile")
	}
	if err := validateManifestMetadata(manifest.GromVersion, manifest.DeploymentProfile); err != nil {
		return err
	}
	if len(manifest.Components) != len(requiredComponents) {
		return fmt.Errorf("backup must declare exactly %d components", len(requiredComponents))
	}
	for i, expected := range requiredComponents {
		component := manifest.Components[i]
		if component.Name != expected.Name || component.File != expected.File || component.Kind != expected.Kind {
			return fmt.Errorf("component %d does not match the format contract", i)
		}
		if component.Bytes < 0 || component.EntryCount < 0 || component.ContentBytes < 0 {
			return fmt.Errorf("component %q has invalid measurements", component.Name)
		}
		if len(component.SHA256) != 64 || !isLowerHex(component.SHA256) {
			return fmt.Errorf("component %q has an invalid checksum", component.Name)
		}
		if component.Kind == "file" &&
			(component.EntryCount != 1 || component.ContentBytes != component.Bytes || component.Bytes > MaxConfigSize) {
			return fmt.Errorf("component %q has invalid file measurements", component.Name)
		}
	}
	return nil
}

func validateManifestMetadata(gromVersion, deploymentProfile string) error {
	if strings.TrimSpace(gromVersion) == "" || len(gromVersion) > 128 {
		return fmt.Errorf("grom version is required")
	}
	switch deploymentProfile {
	case "development", "permissive", "strict":
		return nil
	default:
		return fmt.Errorf("deployment profile is invalid")
	}
}

func readManifest(path string) (Manifest, []byte, error) {
	file, err := os.Open(filepath.Join(path, "manifest.json"))
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open manifest: %w", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, MaxManifestSize+1))
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read manifest: %w", err)
	}
	if len(raw) > MaxManifestSize {
		return Manifest{}, nil, fmt.Errorf("manifest exceeds %d bytes", MaxManifestSize)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Manifest{}, nil, fmt.Errorf("manifest contains trailing data")
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, nil, err
	}
	return manifest, raw, nil
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
