package application

import (
	"github.com/jfxdev/grom/backend/internal/constants"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

func PolicyPresets() []registrydomain.PolicyPreset {
	fourteen := 14
	five := 5
	two := 2
	return []registrydomain.PolicyPreset{
		{
			Key: "protect-production-tags", Name: "Protect production tags", Category: "Protection",
			Description: "Protect stable release references from accidental replacement or cleanup.",
			Outcome:     "Tags stable, prod and v* cannot be overwritten or removed by lifecycle.",
			Policy: registrydomain.Policy{
				Type: constants.RepositoryPolicyTagProtection, Enabled: true,
				TagPatterns: []string{"stable", "prod", "v*"}, PreventOverwrite: true,
				PreventDeletion: true, ExcludeFromLifecycle: true,
			},
		},
		{
			Key: "immutable-releases", Name: "Immutable releases", Category: "Immutability",
			Description: "Keep versioned image references stable after their first push.",
			Outcome:     "Existing v* tags cannot be overwritten.",
			Caution:     "Publishing a corrected release requires a new version tag.",
			Policy: registrydomain.Policy{
				Type: constants.RepositoryPolicyImmutability, Enabled: true,
				TagPatterns: []string{"v*"}, PreventOverwrite: true,
			},
		},
		{
			Key: "clean-temporary-builds", Name: "Clean temporary builds", Category: "Lifecycle",
			Description: "Define a compact retention window for pull requests, branches and snapshots.",
			Outcome:     "Temporary tags expire after 14 days while the latest five are retained.",
			Caution:     "Lifecycle execution is reviewed separately before content is removed.",
			Policy: registrydomain.Policy{
				Type: constants.RepositoryPolicyRetention, Enabled: true,
				TagPatterns:     []string{"pr-*", "branch-*", "snapshot-*"},
				ExpireAfterDays: &fourteen, ExpireAfterDaysEnabled: boolPointer(true),
				KeepLast: &five, KeepLastEnabled: boolPointer(true),
				UntaggedGraceDays: &two, UntaggedGraceDaysEnabled: boolPointer(true),
			},
		},
		{
			Key: "release-tag-convention", Name: "Release tag convention", Category: "Naming",
			Description: "Guide release and development tags toward predictable names.",
			Outcome:     "Manifest tags must match v*, latest, sha-* or branch-*.",
			Policy: registrydomain.Policy{
				Type: constants.RepositoryPolicyTagNaming, Enabled: true,
				AllowedPatterns: []string{"v*", "latest", "sha-*", "branch-*"},
			},
		},
		{
			Key: "confirm-manual-deletion", Name: "Confirm manual deletion", Category: "Deletion",
			Description: "Require an operator-provided reason before a manual artifact deletion.",
			Outcome:     "Manual deletion records why the artifact was removed.",
			Policy: registrydomain.Policy{
				Type: constants.RepositoryPolicyManualDeletion, Enabled: true, RequireReason: true,
			},
		},
	}
}

func boolPointer(value bool) *bool {
	return &value
}
