package main

import (
	"bytes"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryDefaultListenAddressIsLoopback(t *testing.T) {
	address, err := netip.ParseAddrPort(defaultRecoveryListenAddress)
	if err != nil {
		t.Fatal(err)
	}
	if !address.Addr().IsLoopback() {
		t.Fatalf("recovery default must be loopback-only, got %s", address)
	}
}

func TestRunValidatesCommandsAndArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing command"},
		{name: "unknown command", args: []string{"unknown"}},
		{name: "recovery positional argument", args: []string{"recovery", "extra"}},
		{name: "recovery invalid limit", args: []string{"recovery", "--max-upload-bytes=0"}},
		{name: "agent positional argument", args: []string{"agent", "extra"}},
		{name: "agent relative socket", args: []string{"agent", "--socket=relative.sock"}},
		{name: "estimate positional argument", args: []string{"estimate", "extra"}},
		{name: "create missing arguments", args: []string{"create"}},
		{name: "inspect missing path", args: []string{"inspect"}},
		{name: "restore missing arguments", args: []string{"restore"}},
		{name: "invalid flag", args: []string{"inspect", "--unknown"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := run(test.args, &bytes.Buffer{}); err == nil {
				t.Fatal("expected command validation error")
			}
		})
	}
}

func TestRunEstimateCreateInspectAndRestore(t *testing.T) {
	fixture := createCommandFixture(t)
	var estimateOutput bytes.Buffer
	if err := run([]string{
		"estimate",
		"--grom-data=" + fixture.gromData,
		"--signing-certs=" + fixture.signing,
		"--registry-data=" + fixture.registry,
	}, &estimateOutput); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(estimateOutput.String(), "estimated_total_bytes=") ||
		!strings.Contains(estimateOutput.String(), "grom-data_bytes=") {
		t.Fatalf("unexpected estimate output: %s", estimateOutput.String())
	}

	backupRoot := t.TempDir()
	var createOutput bytes.Buffer
	if err := run([]string{
		"create",
		"--destination=" + backupRoot,
		"--grom-data=" + fixture.gromData,
		"--signing-certs=" + fixture.signing,
		"--registry-data=" + fixture.registry,
		"--distribution-config=" + fixture.config,
		"--deployment-profile=development",
	}, &createOutput); err != nil {
		t.Fatal(err)
	}
	backupPath := outputValue(t, createOutput.String(), "backup_path")
	if backupPath == "" {
		t.Fatalf("create output did not include a backup path: %s", createOutput.String())
	}

	var inspectOutput bytes.Buffer
	if err := run([]string{"inspect", "--path=" + backupPath}, &inspectOutput); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inspectOutput.String(), "status=complete") ||
		!strings.Contains(inspectOutput.String(), "grom_version="+version) {
		t.Fatalf("unexpected inspect output: %s", inspectOutput.String())
	}

	targetRoot := t.TempDir()
	gromTarget := makeCommandDir(t, targetRoot, "grom")
	signingTarget := makeCommandDir(t, targetRoot, "signing")
	registryTarget := makeCommandDir(t, targetRoot, "registry")
	configTarget := filepath.Join(targetRoot, "distribution", "config.yml")
	var restoreOutput bytes.Buffer
	if err := run([]string{
		"restore",
		"--path=" + backupPath,
		"--grom-data-target=" + gromTarget,
		"--signing-certs-target=" + signingTarget,
		"--registry-data-target=" + registryTarget,
		"--distribution-config-target=" + configTarget,
	}, &restoreOutput); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restoreOutput.String(), "status=restored") {
		t.Fatalf("unexpected restore output: %s", restoreOutput.String())
	}
	assertCommandFile(t, filepath.Join(gromTarget, "grom.db"), "sqlite")
	assertCommandFile(t, configTarget, "version: 0.1\n")
}

func TestRunPropagatesOutputFailures(t *testing.T) {
	fixture := createCommandFixture(t)
	err := run([]string{
		"estimate",
		"--grom-data=" + fixture.gromData,
		"--signing-certs=" + fixture.signing,
		"--registry-data=" + fixture.registry,
	}, failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "write estimate output") {
		t.Fatalf("expected output failure, got %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type commandFixture struct {
	gromData string
	signing  string
	registry string
	config   string
}

func createCommandFixture(t *testing.T) commandFixture {
	t.Helper()
	root := t.TempDir()
	fixture := commandFixture{
		gromData: makeCommandDir(t, root, "grom"),
		signing:  makeCommandDir(t, root, "signing"),
		registry: makeCommandDir(t, root, "registry"),
		config:   filepath.Join(root, "config.yml"),
	}
	writeCommandFile(t, filepath.Join(fixture.gromData, "grom.db"), "sqlite")
	writeCommandFile(t, filepath.Join(fixture.signing, "signing-key.pem"), "private")
	writeCommandFile(t, filepath.Join(fixture.signing, "signing-cert.pem"), "certificate")
	writeCommandFile(t, filepath.Join(fixture.signing, "jwks.json"), "{}")
	writeCommandFile(t, filepath.Join(fixture.registry, "docker", "blob"), "layer")
	writeCommandFile(t, fixture.config, "version: 0.1\n")
	return fixture
}

func makeCommandDir(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCommandFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertCommandFile(t *testing.T, path, expected string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != expected {
		t.Fatalf("%s = %q, want %q", path, raw, expected)
	}
}

func outputValue(t *testing.T, output, key string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	t.Fatalf("output does not contain %s: %s", key, output)
	return ""
}
