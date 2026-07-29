package backup

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEstimateCountsRegularFilesAndRejectsUnsafeSources(t *testing.T) {
	fixture := createFixture(t)
	results, err := Estimate(map[string]string{
		ComponentGromData:     fixture.gromData,
		ComponentSigningCerts: fixture.signing,
		ComponentRegistryData: fixture.registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || results[0].Name != ComponentGromData || results[0].Bytes != int64(len("sqlite")) {
		t.Fatalf("unexpected estimate: %#v", results)
	}
	if _, err := Estimate(map[string]string{}); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected missing source rejection, got %v", err)
	}
	if err := os.Symlink(
		filepath.Join(fixture.gromData, "grom.db"),
		filepath.Join(fixture.gromData, "link"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := Estimate(map[string]string{
		ComponentGromData:     fixture.gromData,
		ComponentSigningCerts: fixture.signing,
		ComponentRegistryData: fixture.registry,
	}); err == nil || !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("expected unsafe source rejection, got %v", err)
	}
}

func TestValidateManifestRejectsEveryContractViolation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{name: "format", mutate: func(manifest *Manifest) { manifest.FormatVersion = 99 }},
		{name: "backup ID", mutate: func(manifest *Manifest) { manifest.BackupID = "invalid" }},
		{name: "created at", mutate: func(manifest *Manifest) { manifest.CreatedAt = time.Time{} }},
		{name: "consistency", mutate: func(manifest *Manifest) { manifest.Consistency = "crash-consistent" }},
		{name: "version", mutate: func(manifest *Manifest) { manifest.GromVersion = " " }},
		{name: "storage", mutate: func(manifest *Manifest) { manifest.Database = "postgres" }},
		{name: "profile", mutate: func(manifest *Manifest) { manifest.DeploymentProfile = "unknown" }},
		{name: "component count", mutate: func(manifest *Manifest) { manifest.Components = manifest.Components[:3] }},
		{name: "component contract", mutate: func(manifest *Manifest) { manifest.Components[0].Name = "other" }},
		{name: "negative measurements", mutate: func(manifest *Manifest) { manifest.Components[0].Bytes = -1 }},
		{name: "checksum", mutate: func(manifest *Manifest) { manifest.Components[0].SHA256 = "ABC" }},
		{name: "file measurements", mutate: func(manifest *Manifest) { manifest.Components[3].EntryCount = 2 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validTestManifest()
			test.mutate(&manifest)
			if err := validateManifest(manifest); err == nil {
				t.Fatal("expected manifest validation error")
			}
		})
	}

	quiesced := validTestManifest()
	quiesced.FormatVersion = QuiescedFormatVersion
	quiesced.Consistency = "quiesced"
	if err := validateManifest(quiesced); err != nil {
		t.Fatalf("expected valid quiesced manifest: %v", err)
	}
}

func TestReadManifestRejectsMissingOversizedUnknownAndTrailingData(t *testing.T) {
	if _, _, err := readManifest(t.TempDir()); err == nil || !strings.Contains(err.Error(), "open manifest") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}

	for _, test := range []struct {
		name    string
		content []byte
	}{
		{name: "malformed", content: []byte(`{`)},
		{name: "unknown field", content: []byte(`{"unknown":true}`)},
		{name: "trailing data", content: append(manifestJSON(t, validTestManifest()), []byte(` {}`)...)},
		{name: "oversized", content: bytes.Repeat([]byte("x"), MaxManifestSize+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "manifest.json"), test.content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := readManifest(root); err == nil {
				t.Fatal("expected invalid manifest rejection")
			}
		})
	}
}

func TestChecksumFileRejectsMalformedUnsafeAndDuplicateEntries(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	for _, test := range []struct {
		name    string
		content string
	}{
		{name: "short", content: "bad\n"},
		{name: "uppercase hash", content: strings.Repeat("A", 64) + "  manifest.json\n"},
		{name: "unsafe name", content: validHash + "  nested/manifest.json\n"},
		{name: "duplicate", content: validHash + "  manifest.json\n" + validHash + "  manifest.json\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "SHA256SUMS")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readChecksumFile(path); err == nil {
				t.Fatal("expected invalid checksum file rejection")
			}
		})
	}
	if _, err := readChecksumFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing checksum file error")
	}
}

func TestRestoreRejectsVersionPathsAndExistingConfiguration(t *testing.T) {
	fixture := createFixture(t)
	_, backupPath, err := Create(CreateOptions{
		DestinationRoot: t.TempDir(), GromData: fixture.gromData,
		SigningCerts: fixture.signing, RegistryData: fixture.registry,
		DistributionConfig: fixture.config, GromVersion: "test-version",
		DeploymentProfile: "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	validOptions := RestoreOptions{
		BackupPath:               backupPath,
		GromDataTarget:           emptyDir(t, root, "grom"),
		SigningCertsTarget:       emptyDir(t, root, "signing"),
		RegistryDataTarget:       emptyDir(t, root, "registry"),
		DistributionConfigTarget: filepath.Join(root, "configuration", "config.yml"),
		TargetGromVersion:        "test-version",
	}

	for _, test := range []struct {
		name   string
		mutate func(*RestoreOptions)
	}{
		{name: "version", mutate: func(options *RestoreOptions) { options.TargetGromVersion = "other" }},
		{name: "relative volume", mutate: func(options *RestoreOptions) { options.GromDataTarget = "relative" }},
		{name: "relative config", mutate: func(options *RestoreOptions) { options.DistributionConfigTarget = "relative" }},
		{name: "existing config", mutate: func(options *RestoreOptions) {
			options.DistributionConfigTarget = filepath.Join(root, "existing.yml")
			if err := os.WriteFile(options.DistributionConfigTarget, []byte("existing"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := validOptions
			test.mutate(&options)
			if _, err := Restore(options); err == nil {
				t.Fatal("expected restore option rejection")
			}
		})
	}
}

func TestRestoreMarkerValidationAndConsumption(t *testing.T) {
	root := t.TempDir()
	marker, path, err := ReadRestoreMarker(root)
	if err != nil || marker.BackupID != "" || filepath.Base(path) != RestoreMarkerName {
		t.Fatalf("unexpected missing marker result: marker=%#v path=%s error=%v", marker, path, err)
	}

	if err := os.WriteFile(path, []byte(`not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRestoreMarker(root); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected malformed marker rejection, got %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"backupId":"invalid","sourceVersion":"v1","restoredAt":"2026-07-29T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRestoreMarker(root); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected semantic marker rejection, got %v", err)
	}

	expected := RestoreMarker{
		BackupID: uuid.NewString(), SourceVersion: "v1",
		RestoredAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	raw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	actual, actualPath, err := ReadRestoreMarker(root)
	if err != nil || actual != expected || actualPath != path {
		t.Fatalf("unexpected valid marker: marker=%#v path=%s error=%v", actual, actualPath, err)
	}
	if err := ConsumeRestoreMarker(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("restore marker remains after consumption: %v", err)
	}
	if err := ConsumeRestoreMarker(path); err == nil {
		t.Fatal("expected missing marker consumption error")
	}
}

func TestUnbundleRejectsInvalidShapeLimitsAndDuplicateSets(t *testing.T) {
	if _, err := Unbundle("relative", strings.NewReader(""), 1); err == nil {
		t.Fatal("expected relative destination rejection")
	}
	if _, err := Unbundle(t.TempDir(), strings.NewReader(""), 0); err == nil {
		t.Fatal("expected invalid limit rejection")
	}

	tests := []struct {
		name    string
		entries []bundleTestEntry
		max     int64
	}{
		{name: "unsupported type", entries: []bundleTestEntry{{name: "grom-backup-a/manifest.json", kind: tar.TypeDir}}, max: 100},
		{name: "unsafe path", entries: []bundleTestEntry{{name: "../manifest.json", content: "x"}}, max: 100},
		{name: "multiple sets", entries: []bundleTestEntry{
			{name: "grom-backup-a/manifest.json", content: "x"},
			{name: "grom-backup-b/COMPLETE", content: ""},
		}, max: 100},
		{name: "undeclared", entries: []bundleTestEntry{{name: "grom-backup-a/secret", content: "x"}}, max: 100},
		{name: "duplicate", entries: []bundleTestEntry{
			{name: "grom-backup-a/manifest.json", content: "x"},
			{name: "grom-backup-a/manifest.json", content: "x"},
		}, max: 100},
		{name: "limit", entries: []bundleTestEntry{{name: "grom-backup-a/manifest.json", content: "too large"}}, max: 1},
		{name: "incomplete", entries: []bundleTestEntry{{name: "grom-backup-a/manifest.json", content: "x"}}, max: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Unbundle(t.TempDir(), bytes.NewReader(testBundle(t, test.entries)), test.max); err == nil {
				t.Fatal("expected invalid bundle rejection")
			}
		})
	}
}

func TestPaginateRejectsMalformedAndInvalidSummaryCursors(t *testing.T) {
	validID := uuid.NewString()
	for _, cursor := range []string{
		base64.RawURLEncoding.EncodeToString([]byte(`{}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"createdAt":"bad","backupId":"` + validID + `"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"createdAt":"2026-07-29T00:00:00Z","backupId":"invalid"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"createdAt":"2026-07-29T00:00:00Z","backupId":"` + validID + `"} {}`)),
	} {
		if _, err := Paginate(nil, cursor); !errorsIsInvalidCursor(err) {
			t.Fatalf("expected invalid cursor for %q, got %v", cursor, err)
		}
	}

	cursorRaw, err := json.Marshal(pageCursor{
		CreatedAt: "2026-07-29T00:00:00Z",
		BackupID:  validID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Paginate([]Summary{{
		BackupID: uuid.NewString(), CreatedAt: "invalid",
	}}, base64.RawURLEncoding.EncodeToString(cursorRaw)); !errorsIsInvalidCursor(err) {
		t.Fatalf("expected invalid summary timestamp rejection, got %v", err)
	}
}

func validTestManifest() Manifest {
	components := make([]Component, len(requiredComponents))
	for index, expected := range requiredComponents {
		components[index] = expected
		components[index].Bytes = 1
		components[index].SHA256 = strings.Repeat("a", 64)
		if expected.Kind == "file" {
			components[index].EntryCount = 1
			components[index].ContentBytes = 1
		}
	}
	return Manifest{
		FormatVersion: FormatVersion, BackupID: uuid.NewString(),
		CreatedAt:   time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Consistency: "offline", GromVersion: "v1", Database: "sqlite",
		RegistryStorage: "filesystem", DeploymentProfile: "development",
		Components: components,
	}
}

func manifestJSON(t *testing.T, manifest Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type bundleTestEntry struct {
	name    string
	content string
	kind    byte
}

func testBundle(t *testing.T, entries []bundleTestEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Typeflag: kind, Mode: 0o600}
		if kind == tar.TypeReg {
			header.Size = int64(len(entry.content))
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := writer.Write([]byte(entry.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func errorsIsInvalidCursor(err error) bool {
	return err == ErrInvalidCursor
}
