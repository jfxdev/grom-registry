package application

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
)

func TestTokenServiceSubjectReturnsVerifiedRegistrySubject(t *testing.T) {
	signer, err := signing.LoadOrCreate(filepath.Join(t.TempDir(), "key.pem"), filepath.Join(t.TempDir(), "cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewTokenService(nil, nil, signer, time.Minute)
	raw, err := service.IssueInternal("release-bot", nil)
	if err != nil {
		t.Fatal(err)
	}
	if subject, ok := service.Subject(raw); !ok || subject != "release-bot" {
		t.Fatalf("expected verified release-bot subject, got %q, %t", subject, ok)
	}
	if subject, ok := service.Subject("not-a-token"); ok || subject != "" {
		t.Fatalf("expected invalid token to have no subject, got %q, %t", subject, ok)
	}
}
