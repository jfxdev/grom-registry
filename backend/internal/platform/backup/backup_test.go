package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAgentCreatesListsDownloadsAndImportsQuiescedBackup(t *testing.T) {
	fixture := createFixture(t)
	root := t.TempDir()
	socketRoot, err := os.MkdirTemp("/tmp", "grom-agent-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketRoot) })
	socket := filepath.Join(socketRoot, "agent.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errorsCh := make(chan error, 1)
	go func() {
		errorsCh <- ServeAgent(ctx, AgentOptions{
			SocketPath: socket, DestinationRoot: root,
			GromData: fixture.gromData, SigningCerts: fixture.signing,
			RegistryData: fixture.registry, DistributionConfig: fixture.config,
		})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case err := <-errorsCh:
			if errors.Is(err, syscall.EPERM) {
				t.Skip("sandbox does not permit Unix sockets")
			}
			t.Fatalf("agent exited before readiness: %v", err)
		default:
		}
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent socket did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	client := NewAgentClient(socket)
	summary, err := client.Create(context.Background(), AgentCreateRequest{
		GromVersion: "test-version", DeploymentProfile: "development", Consistency: "quiesced",
	})
	if err != nil {
		t.Fatalf("create through agent: %v", err)
	}
	summaries, err := client.List(context.Background())
	if err != nil || len(summaries) != 1 || summaries[0].BackupID != summary.BackupID {
		t.Fatalf("list agent backups: summaries=%#v error=%v", summaries, err)
	}
	response, err := client.Download(context.Background(), summary.BackupID)
	if err != nil {
		t.Fatalf("download agent backup: %v", err)
	}
	var bundle bytes.Buffer
	_, _ = bundle.ReadFrom(response.Body)
	_ = response.Body.Close()
	imported, err := Unbundle(t.TempDir(), bytes.NewReader(bundle.Bytes()), 1<<20)
	if err != nil {
		t.Fatalf("import downloaded bundle: %v", err)
	}
	inspection, err := Inspect(imported.Path)
	if err != nil || inspection.Manifest.FormatVersion != QuiescedFormatVersion {
		t.Fatalf("inspect imported quiesced backup: inspection=%#v error=%v", inspection, err)
	}
	if err := client.Delete(context.Background(), summary.BackupID); err != nil {
		t.Fatalf("delete through agent: %v", err)
	}
	summaries, err = client.List(context.Background())
	if err != nil || len(summaries) != 0 {
		t.Fatalf("list after agent deletion: summaries=%#v error=%v", summaries, err)
	}
	cancel()
	if err := <-errorsCh; err != nil {
		t.Fatalf("stop agent: %v", err)
	}
}

func TestPaginateAndDeleteBackups(t *testing.T) {
	fixture := createFixture(t)
	root := t.TempDir()
	for index := 0; index < 7; index++ {
		_, _, err := Create(CreateOptions{
			DestinationRoot: root, GromData: fixture.gromData, SigningCerts: fixture.signing,
			RegistryData: fixture.registry, DistributionConfig: fixture.config,
			GromVersion: "test-version", DeploymentProfile: "development",
			Now: func() time.Time {
				return time.Date(2026, 7, 29, 16, index, 0, 0, time.UTC)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	summaries, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Paginate(summaries, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 5 || first.Total != 7 || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", first)
	}
	second, err := Paginate(summaries, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.Total != 7 || second.NextCursor != "" {
		t.Fatalf("unexpected second page: %#v", second)
	}
	if _, err := Paginate(summaries, "not-a-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected invalid cursor, got %v", err)
	}
	deleted := first.Items[0]
	if _, err := Delete(root, deleted.BackupID); err != nil {
		t.Fatal(err)
	}
	remaining, err := List(root)
	if err != nil || len(remaining) != 6 {
		t.Fatalf("list after deletion: summaries=%d error=%v", len(remaining), err)
	}
	if _, err := findSummary(root, deleted.BackupID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted backup remains visible: %v", err)
	}
}

func TestCreateInspectAndRestore(t *testing.T) {
	fixture := createFixture(t)
	backupRoot := filepath.Join(t.TempDir(), "backup root")
	inspection, backupPath, err := Create(CreateOptions{
		DestinationRoot: backupRoot,
		GromData:        fixture.gromData, SigningCerts: fixture.signing,
		RegistryData: fixture.registry, DistributionConfig: fixture.config,
		GromVersion: "test-version", DeploymentProfile: "development",
		Now: func() time.Time { return time.Date(2026, 7, 29, 16, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if inspection.Manifest.FormatVersion != FormatVersion || inspection.TotalBytes == 0 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if mode := fileMode(t, backupPath); mode != 0o700 {
		t.Fatalf("backup directory mode = %o, want 700", mode)
	}
	for _, name := range []string{"manifest.json", "SHA256SUMS", "COMPLETE", "grom-data.tar"} {
		if mode := fileMode(t, filepath.Join(backupPath, name)); mode != 0o600 {
			t.Fatalf("%s mode = %o, want 600", name, mode)
		}
	}
	if _, err := Inspect(backupPath); err != nil {
		t.Fatalf("inspect completed backup: %v", err)
	}

	targetRoot := t.TempDir()
	gromTarget := emptyDir(t, targetRoot, "grom")
	signingTarget := emptyDir(t, targetRoot, "signing")
	registryTarget := emptyDir(t, targetRoot, "registry")
	configTarget := filepath.Join(targetRoot, "configuration", "config.yml")
	if _, err := Restore(RestoreOptions{
		BackupPath: backupPath, GromDataTarget: gromTarget,
		SigningCertsTarget: signingTarget, RegistryDataTarget: registryTarget,
		DistributionConfigTarget: configTarget, TargetGromVersion: "test-version",
	}); err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	assertFileContent(t, filepath.Join(gromTarget, "grom.db"), "sqlite")
	assertFileContent(t, filepath.Join(registryTarget, "docker", "blob"), "layer")
	assertFileContent(t, configTarget, "version: 0.1\n")
	marker, _, err := ReadRestoreMarker(gromTarget)
	if err != nil || marker.BackupID != inspection.Manifest.BackupID {
		t.Fatalf("read restore marker: marker=%#v error=%v", marker, err)
	}
}

func TestBundleRoundTripRejectsUnexpectedOuterEntries(t *testing.T) {
	fixture := createFixture(t)
	root := t.TempDir()
	inspection, _, err := Create(CreateOptions{
		DestinationRoot: root, GromData: fixture.gromData, SigningCerts: fixture.signing,
		RegistryData: fixture.registry, DistributionConfig: fixture.config,
		GromVersion: "test-version", DeploymentProfile: "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	var bundle bytes.Buffer
	if _, err := Bundle(root, inspection.Manifest.BackupID, &bundle); err != nil {
		t.Fatal(err)
	}
	imported, err := Unbundle(t.TempDir(), bytes.NewReader(bundle.Bytes()), 1<<20)
	if err != nil {
		t.Fatalf("unbundle valid backup: %v", err)
	}
	if _, err := Inspect(imported.Path); err != nil {
		t.Fatalf("inspect imported backup: %v", err)
	}

	var malicious bytes.Buffer
	writer := tar.NewWriter(&malicious)
	_ = writer.WriteHeader(&tar.Header{Name: "../escape", Typeflag: tar.TypeReg, Size: 1, Mode: 0o600})
	_, _ = writer.Write([]byte("x"))
	_ = writer.Close()
	if _, err := Unbundle(t.TempDir(), bytes.NewReader(malicious.Bytes()), 1<<20); err == nil {
		t.Fatal("expected unsafe outer bundle rejection")
	}
}

func TestInspectRejectsIncompleteCorruptAndUnexpectedSets(t *testing.T) {
	fixture := createFixture(t)
	_, backupPath, err := Create(CreateOptions{
		DestinationRoot: t.TempDir(), GromData: fixture.gromData,
		SigningCerts: fixture.signing, RegistryData: fixture.registry,
		DistributionConfig: fixture.config, GromVersion: "test-version",
		DeploymentProfile: "strict",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(backupPath, "COMPLETE")); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(backupPath); err == nil || !strings.Contains(err.Error(), "completion") {
		t.Fatalf("expected incomplete-set rejection, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupPath, "COMPLETE"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupPath, "unexpected"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(backupPath); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("expected unexpected-file rejection, got %v", err)
	}
	if err := os.Remove(filepath.Join(backupPath, "unexpected")); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(backupPath, "registry-data.tar"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("corruption"))
	_ = file.Close()
	if _, err := Inspect(backupPath); err == nil {
		t.Fatal("expected corrupt component rejection")
	}
}

func TestCreateRejectsSymlinkAndCleansPartialSet(t *testing.T) {
	fixture := createFixture(t)
	if err := os.Symlink(filepath.Join(fixture.gromData, "grom.db"), filepath.Join(fixture.gromData, "link")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, _, err := Create(CreateOptions{
		DestinationRoot: root, GromData: fixture.gromData, SigningCerts: fixture.signing,
		RegistryData: fixture.registry, DistributionConfig: fixture.config,
		GromVersion: "test-version", DeploymentProfile: "development",
	}); err == nil {
		t.Fatal("expected symbolic-link rejection")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed create left files behind: %v", entries)
	}
}

func TestRestoreRejectsNonEmptyTargetBeforeExtraction(t *testing.T) {
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
	gromTarget := emptyDir(t, root, "grom")
	writeFixtureFile(t, filepath.Join(gromTarget, "existing"), "do not overwrite", 0o600)
	errTarget := filepath.Join(root, "recovered.yml")
	_, err = Restore(RestoreOptions{
		BackupPath: backupPath, GromDataTarget: gromTarget,
		SigningCertsTarget:       emptyDir(t, root, "signing"),
		RegistryDataTarget:       emptyDir(t, root, "registry"),
		DistributionConfigTarget: errTarget, TargetGromVersion: "test-version",
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected non-empty target rejection, got %v", err)
	}
	assertFileContent(t, filepath.Join(gromTarget, "existing"), "do not overwrite")
}

func TestExtractArchiveRejectsTraversalAndSpecialFiles(t *testing.T) {
	for _, test := range []struct {
		name, path string
		kind       byte
	}{
		{name: "traversal", path: "../escaped", kind: tar.TypeReg},
		{name: "absolute", path: "/escaped", kind: tar.TypeReg},
		{name: "symlink", path: "link", kind: tar.TypeSymlink},
		{name: "device", path: "device", kind: tar.TypeChar},
	} {
		t.Run(test.name, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), "malicious.tar")
			file, err := os.Create(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			writer := tar.NewWriter(file)
			header := &tar.Header{Name: test.path, Typeflag: test.kind, Mode: 0o600}
			if test.kind == tar.TypeReg {
				header.Size = 1
			}
			if err := writer.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			if test.kind == tar.TypeReg {
				_, _ = writer.Write([]byte("x"))
			}
			_ = writer.Close()
			_ = file.Close()
			err = extractArchive(archivePath, t.TempDir(), Component{EntryCount: 1, ContentBytes: 1})
			if err == nil {
				t.Fatal("expected unsafe archive rejection")
			}
		})
	}
}

type fixturePaths struct {
	gromData, signing, registry, config string
}

func createFixture(t *testing.T) fixturePaths {
	t.Helper()
	root := t.TempDir()
	fixture := fixturePaths{
		gromData: emptyDir(t, root, "grom-data"),
		signing:  emptyDir(t, root, "signing"),
		registry: emptyDir(t, root, "registry"),
		config:   filepath.Join(root, "config.yml"),
	}
	writeFixtureFile(t, filepath.Join(fixture.gromData, "grom.db"), "sqlite", 0o640)
	writeFixtureFile(t, filepath.Join(fixture.signing, "signing-key.pem"), "private", 0o600)
	writeFixtureFile(t, filepath.Join(fixture.signing, "signing-cert.pem"), "certificate", 0o644)
	writeFixtureFile(t, filepath.Join(fixture.signing, "jwks.json"), "{}", 0o644)
	writeFixtureFile(t, filepath.Join(fixture.registry, "docker", "blob"), "layer", 0o640)
	writeFixtureFile(t, fixture.config, "version: 0.1\n", 0o644)
	return fixture
}

func emptyDir(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != expected {
		t.Fatalf("%s = %q, want %q", path, raw, expected)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
