package signing

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrCreatePropagatesNonMissingKeyReadError(t *testing.T) {
	keyPath := t.TempDir()
	certPath := filepath.Join(t.TempDir(), "cert.pem")

	_, err := LoadOrCreate(keyPath, certPath)
	if err == nil || !strings.Contains(err.Error(), "read signing key") {
		t.Fatalf("expected signing key read error, got %v", err)
	}
}

func TestProbeExposesOnlySigningMetadata(t *testing.T) {
	signer, err := LoadOrCreate(filepath.Join(t.TempDir(), "key.pem"), filepath.Join(t.TempDir(), "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	algorithm, keyID, err := signer.Probe()
	if err != nil {
		t.Fatal(err)
	}
	if algorithm != "RS256" || keyID != KeyID {
		t.Fatalf("unexpected signing metadata: algorithm=%q keyID=%q", algorithm, keyID)
	}
	if _, _, err := (*Signer)(nil).Probe(); err == nil {
		t.Fatal("expected an unavailable signer to fail its probe")
	}
}
