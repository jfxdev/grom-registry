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
