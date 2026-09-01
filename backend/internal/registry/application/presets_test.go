package application

import "testing"

func TestPolicyPresetsEnableIndependentRetentionCriteria(t *testing.T) {
	var retentionFound bool
	for _, preset := range PolicyPresets() {
		if preset.Key != "clean-temporary-builds" {
			continue
		}
		retentionFound = true
		policy := preset.Policy
		if policy.ExpireAfterDays == nil || *policy.ExpireAfterDays != 14 || policy.ExpireAfterDaysEnabled == nil || !*policy.ExpireAfterDaysEnabled {
			t.Fatalf("expected enabled 14-day expiration policy, got %#v", policy)
		}
		if policy.KeepLast == nil || *policy.KeepLast != 5 || policy.KeepLastEnabled == nil || !*policy.KeepLastEnabled {
			t.Fatalf("expected enabled keep-last policy, got %#v", policy)
		}
		if policy.UntaggedGraceDays == nil || *policy.UntaggedGraceDays != 2 || policy.UntaggedGraceDaysEnabled == nil || !*policy.UntaggedGraceDaysEnabled {
			t.Fatalf("expected enabled untagged grace policy, got %#v", policy)
		}
	}
	if !retentionFound {
		t.Fatal("missing clean-temporary-builds preset")
	}
}
