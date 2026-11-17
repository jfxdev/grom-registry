package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/identity/infrastructure/password"
)

func countingVerifier(service *Service) *int {
	calls := 0
	service.verifySecret = func(hash, secret string) bool {
		calls++
		return password.Verify(hash, secret)
	}
	return &calls
}

func TestAPIKeyVerificationCacheSkipsRepeatedArgon2ButKeepsRevocationImmediate(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	calls := countingVerifier(service)
	ctx := context.Background()
	account, err := service.CreateServiceAccount(ctx, "Agent", "agent", "")
	if err != nil {
		t.Fatal(err)
	}
	key, err := service.CreateServiceAccountAPIToken(ctx, account.ID, "mcp", nil)
	if err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if _, err := service.AuthenticateAPIToken(ctx, key.Secret); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.AuthenticateRegistry(ctx, account.Username, key.Secret); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("argon2 verifications = %d, want 1 within the cache TTL", *calls)
	}

	publicID := strings.SplitN(key.Secret, "_", 3)[1]
	if _, err := service.AuthenticateAPIToken(ctx, "grm_"+publicID+"_wrong"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected a wrong secret to fail after a cached success, got %v", err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, key.Secret); err != nil {
		t.Fatalf("a failed attempt must not evict the valid entry: %v", err)
	}

	if err := service.RevokeServiceAccountAPIToken(ctx, account.ID, key.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, key.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected revocation to apply within the cache TTL, got %v", err)
	}
}

func TestAPIKeyVerificationCacheRejectsDisabledOwnerWithinTTL(t *testing.T) {
	repository := newFakeRepository()
	service := New(repository, time.Hour)
	ctx := context.Background()
	account, err := service.CreateServiceAccount(ctx, "Agent", "agent", "")
	if err != nil {
		t.Fatal(err)
	}
	key, err := service.CreateServiceAccountAPIToken(ctx, account.ID, "mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, key.Secret); err != nil {
		t.Fatal(err)
	}
	delete(repository.serviceAccounts, account.ID)
	if _, err := service.AuthenticateAPIToken(ctx, key.Secret); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected a removed owner to fail within the cache TTL, got %v", err)
	}
}

func TestAPIKeyVerificationCacheExpiresAndRebindsToTheStoredHash(t *testing.T) {
	cache := newAPIKeyVerificationCache(time.Minute, 2)
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	calls := 0
	verifier := func(hash, secret string) bool {
		calls++
		return hash == "hash:"+secret
	}

	for range 2 {
		if !cache.verify("token", "hash:s", "s", verifier) {
			t.Fatal("expected the secret to verify")
		}
	}
	if calls != 1 {
		t.Fatalf("expected one verification and a cache hit, calls = %d", calls)
	}
	if cache.verify("token", "hash:other", "s", verifier) {
		t.Fatal("a cached entry must not validate against a different stored hash")
	}
	now = now.Add(time.Minute)
	calls = 0
	if !cache.verify("token", "hash:s", "s", verifier) || calls != 1 {
		t.Fatalf("expected an expired entry to be verified again, calls = %d", calls)
	}
}

func TestAPIKeyVerificationCacheIsBounded(t *testing.T) {
	cache := newAPIKeyVerificationCache(time.Minute, 2)
	verifier := func(hash, secret string) bool { return hash == secret }
	for _, id := range []string{"a", "b", "c", "d"} {
		if !cache.verify(id, id, id, verifier) {
			t.Fatalf("verify %s failed", id)
		}
	}
	if len(cache.entries) > 2 {
		t.Fatalf("cache holds %d entries, want at most 2", len(cache.entries))
	}
	if _, ok := cache.entries["d"]; !ok {
		t.Fatal("expected the newest verification to be cached")
	}
}
