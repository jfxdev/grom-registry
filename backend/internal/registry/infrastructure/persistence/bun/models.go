package bunstore

import (
	"time"

	"github.com/uptrace/bun"
)

type repositoryModel struct {
	bun.BaseModel      `bun:"table:registry_repositories,alias:rr"`
	ID                 string     `bun:"id,pk"`
	ProjectID          string     `bun:"project_id,notnull"`
	Name               string     `bun:"name,notnull"`
	Description        string     `bun:"description,notnull"`
	Status             string     `bun:"status,notnull"`
	CreationSource     string     `bun:"creation_source,notnull"`
	Profile            string     `bun:"profile,notnull"`
	ProfileSource      string     `bun:"profile_source,notnull"`
	ProfileConfidence  string     `bun:"profile_confidence,notnull"`
	ProfileInferredAt  *time.Time `bun:"profile_inferred_at"`
	ProfileNeedsReview bool       `bun:"profile_needs_review,notnull"`
	PolicyVersion      int        `bun:"policy_version,notnull"`
	CreatedAt          time.Time  `bun:"created_at,notnull"`
	UpdatedAt          time.Time  `bun:"updated_at,notnull"`
}

type policyModel struct {
	bun.BaseModel     `bun:"table:repository_policies,alias:rp"`
	ID                string    `bun:"id,pk"`
	RepositoryID      string    `bun:"repository_id,notnull"`
	Type              string    `bun:"type,notnull"`
	ConfigurationJSON string    `bun:"configuration_json,notnull"`
	Enabled           bool      `bun:"enabled,notnull"`
	Version           int       `bun:"version,notnull"`
	CreatedAt         time.Time `bun:"created_at,notnull"`
	UpdatedAt         time.Time `bun:"updated_at,notnull"`
}

type manifestModel struct {
	bun.BaseModel            `bun:"table:registry_manifests,alias:rm"`
	ID                       string     `bun:"id,pk"`
	RepositoryID             string     `bun:"repository_id,notnull"`
	Digest                   string     `bun:"digest,notnull"`
	MediaType                string     `bun:"media_type,notnull"`
	ArtifactType             string     `bun:"artifact_type,notnull"`
	SubjectDigest            string     `bun:"subject_digest,notnull"`
	ObservedKind             string     `bun:"observed_kind,notnull"`
	ArtifactRelationship     string     `bun:"artifact_relationship,notnull"`
	ClassificationSource     string     `bun:"classification_source,notnull"`
	ClassificationConfidence string     `bun:"classification_confidence,notnull"`
	ManifestSize             int64      `bun:"manifest_size,notnull"`
	State                    string     `bun:"state,notnull"`
	FirstSeenAt              time.Time  `bun:"first_seen_at,notnull"`
	LastPushedAt             *time.Time `bun:"last_pushed_at"`
	LastSeenAt               time.Time  `bun:"last_seen_at,notnull"`
	UntaggedAt               *time.Time `bun:"untagged_at"`
	DeletedAt                *time.Time `bun:"deleted_at"`
}

type tagModel struct {
	bun.BaseModel `bun:"table:registry_tags,alias:rt"`
	RepositoryID  string     `bun:"repository_id,pk"`
	Name          string     `bun:"name,pk"`
	ManifestID    string     `bun:"manifest_id,notnull"`
	FirstSeenAt   time.Time  `bun:"first_seen_at,notnull"`
	LastMovedAt   time.Time  `bun:"last_moved_at,notnull"`
	LastSeenAt    time.Time  `bun:"last_seen_at,notnull"`
	DetachedAt    *time.Time `bun:"detached_at"`
}

type manifestPlatformModel struct {
	bun.BaseModel  `bun:"table:registry_manifest_platforms,alias:rmp"`
	ManifestID     string `bun:"manifest_id,pk"`
	Digest         string `bun:"digest,pk"`
	OS             string `bun:"os,notnull"`
	Architecture   string `bun:"architecture,notnull"`
	Variant        string `bun:"variant,notnull"`
	CompressedSize int64  `bun:"compressed_size,notnull"`
}

type artifactDeletionModel struct {
	bun.BaseModel `bun:"table:artifact_deletions,alias:ad"`
	ID            string     `bun:"id,pk"`
	RepositoryID  string     `bun:"repository_id,notnull"`
	Digest        string     `bun:"digest,notnull"`
	AffectedTags  string     `bun:"affected_tags_json,notnull"`
	ActorID       string     `bun:"actor_id,notnull"`
	Reason        string     `bun:"reason,notnull"`
	Status        string     `bun:"status,notnull"`
	Message       string     `bun:"message,notnull"`
	StartedAt     time.Time  `bun:"started_at,notnull"`
	CompletedAt   *time.Time `bun:"completed_at"`
}

type lifecyclePreviewModel struct {
	bun.BaseModel      `bun:"table:lifecycle_previews,alias:lp"`
	ID                 string    `bun:"id,pk"`
	RepositoryID       string    `bun:"repository_id,notnull"`
	PolicySnapshotJSON string    `bun:"policy_snapshot_json,notnull"`
	PolicyVersion      int       `bun:"policy_version,notnull"`
	EvaluatorVersion   int       `bun:"evaluator_version,notnull"`
	Status             string    `bun:"status,notnull"`
	InventoryAt        time.Time `bun:"inventory_at,notnull"`
	CreatedBy          string    `bun:"created_by,notnull"`
	CreatedAt          time.Time `bun:"created_at,notnull"`
	ExpiresAt          time.Time `bun:"expires_at,notnull"`
}

type lifecyclePreviewItemModel struct {
	bun.BaseModel    `bun:"table:lifecycle_preview_items,alias:lpi"`
	ID               string    `bun:"id,pk"`
	PreviewID        string    `bun:"preview_id,notnull"`
	ManifestID       string    `bun:"manifest_id,notnull"`
	Digest           string    `bun:"digest,notnull"`
	MediaType        string    `bun:"media_type,notnull"`
	ArtifactType     string    `bun:"artifact_type,notnull"`
	Decision         string    `bun:"decision,notnull"`
	ReasonsJSON      string    `bun:"reasons_json,notnull"`
	ExpectedTagsJSON string    `bun:"expected_tags_json,notnull"`
	CreatedAt        time.Time `bun:"created_at,notnull"`
}

type lifecycleRunModel struct {
	bun.BaseModel `bun:"table:lifecycle_runs,alias:lr"`
	ID            string     `bun:"id,pk"`
	PreviewID     string     `bun:"preview_id,notnull"`
	RepositoryID  string     `bun:"repository_id,notnull"`
	ActorID       string     `bun:"actor_id,notnull"`
	Reason        string     `bun:"reason,notnull"`
	Status        string     `bun:"status,notnull"`
	StartedAt     time.Time  `bun:"started_at,notnull"`
	CompletedAt   *time.Time `bun:"completed_at"`
}

type lifecycleRunItemModel struct {
	bun.BaseModel `bun:"table:lifecycle_run_items,alias:lri"`
	ID            string    `bun:"id,pk"`
	RunID         string    `bun:"run_id,notnull"`
	Digest        string    `bun:"digest,notnull"`
	Status        string    `bun:"status,notnull"`
	Message       string    `bun:"message,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull"`
	UpdatedAt     time.Time `bun:"updated_at,notnull"`
}

type lifecycleLockModel struct {
	bun.BaseModel `bun:"table:lifecycle_locks,alias:ll"`
	RepositoryID  string    `bun:"repository_id,pk"`
	RunID         string    `bun:"run_id,notnull"`
	AcquiredAt    time.Time `bun:"acquired_at,notnull"`
}
