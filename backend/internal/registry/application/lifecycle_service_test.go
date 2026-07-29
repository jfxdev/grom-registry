package application

import (
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

func TestEvaluateLifecycleProtectsAliasesAndReferrers(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	fourteen, one, two := 14, 1, 2
	policies := []registrydomain.Policy{{
		Type: constants.RepositoryPolicyRetention, Enabled: true,
		TagPatterns: []string{"pr-*"}, ExpireAfterDays: &fourteen,
		KeepLast: &one, UntaggedGraceDays: &two,
	}, {
		Type: constants.RepositoryPolicyTagProtection, Enabled: true,
		TagPatterns: []string{"prod"}, PreventDeletion: true, ExcludeFromLifecycle: true,
	}}
	at := func(days int) *time.Time {
		value := now.Add(-time.Duration(days) * 24 * time.Hour)
		return &value
	}
	untagged := now.Add(-3 * 24 * time.Hour)
	inventory := []registrydomain.ManifestInventory{
		{ID: foundation.NewID(), Digest: "sha256:new", Tags: []string{"pr-new"}, State: constants.InventoryStateActive, FirstSeenAt: *at(1), LastPushedAt: at(1)},
		{ID: foundation.NewID(), Digest: "sha256:old", Tags: []string{"pr-old"}, State: constants.InventoryStateActive, FirstSeenAt: *at(30), LastPushedAt: at(30)},
		{ID: foundation.NewID(), Digest: "sha256:protected", Tags: []string{"pr-prod", "prod"}, State: constants.InventoryStateActive, FirstSeenAt: *at(40), LastPushedAt: at(40)},
		{ID: foundation.NewID(), Digest: "sha256:untagged", Tags: []string{}, State: constants.InventoryStateUntagged, FirstSeenAt: *at(10), UntaggedAt: &untagged},
		{ID: foundation.NewID(), Digest: "sha256:subject", Tags: []string{"pr-subject"}, State: constants.InventoryStateActive, FirstSeenAt: *at(50), LastPushedAt: at(50)},
		{ID: foundation.NewID(), Digest: "sha256:sbom", SubjectDigest: "sha256:subject", Tags: []string{}, State: constants.InventoryStateUntagged, FirstSeenAt: *at(10), UntaggedAt: &untagged},
	}

	items, err := evaluateLifecycle(policies, inventory, now)
	if err != nil {
		t.Fatal(err)
	}
	decisions := map[string]string{}
	for _, item := range items {
		decisions[item.Digest] = item.Decision
	}
	expected := map[string]string{
		"sha256:new":       constants.LifecycleDecisionRetained,
		"sha256:old":       constants.LifecycleDecisionEligible,
		"sha256:protected": constants.LifecycleDecisionBlocked,
		"sha256:untagged":  constants.LifecycleDecisionEligible,
		"sha256:subject":   constants.LifecycleDecisionBlocked,
		"sha256:sbom":      constants.LifecycleDecisionBlocked,
	}
	for digest, decision := range expected {
		if decisions[digest] != decision {
			t.Fatalf("%s: expected %s, got %s", digest, decision, decisions[digest])
		}
	}
}

func TestEvaluateLifecycleRequiresRetentionPolicy(t *testing.T) {
	if _, err := evaluateLifecycle(nil, nil, time.Now()); err == nil {
		t.Fatal("expected missing retention policy to fail")
	}
}
