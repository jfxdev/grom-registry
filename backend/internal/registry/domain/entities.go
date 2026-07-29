package domain

import (
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
)

type Repository struct {
	ID                 foundation.ID `json:"id"`
	ProjectID          foundation.ID `json:"projectId"`
	Name               string        `json:"name"`
	Description        string        `json:"description"`
	Status             string        `json:"status"`
	CreationSource     string        `json:"creationSource"`
	Profile            string        `json:"profile"`
	ProfileSource      string        `json:"profileSource"`
	ProfileConfidence  string        `json:"profileConfidence"`
	ProfileInferredAt  *time.Time    `json:"profileInferredAt,omitempty"`
	ProfileNeedsReview bool          `json:"profileNeedsReview"`
	PolicyVersion      int           `json:"policyVersion"`
	Policies           []Policy      `json:"policies"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type Policy struct {
	ID                   foundation.ID `json:"id"`
	RepositoryID         foundation.ID `json:"-"`
	Type                 string        `json:"type"`
	Enabled              bool          `json:"enabled"`
	TagPatterns          []string      `json:"tagPatterns,omitempty"`
	PreventOverwrite     bool          `json:"preventOverwrite"`
	PreventDeletion      bool          `json:"preventDeletion"`
	ExcludeFromLifecycle bool          `json:"excludeFromLifecycle"`
	ExpireAfterDays      *int          `json:"expireAfterDays,omitempty"`
	KeepLast             *int          `json:"keepLast,omitempty"`
	UntaggedGraceDays    *int          `json:"untaggedGraceDays,omitempty"`
	AllowedPatterns      []string      `json:"allowedPatterns,omitempty"`
	RequireReason        bool          `json:"requireReason"`
	Version              int           `json:"version"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}

type PolicyPreset struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Outcome     string `json:"outcome"`
	Caution     string `json:"caution,omitempty"`
	Policy      Policy `json:"policy"`
}

type ArtifactDeletionPreview struct {
	Repository       string   `json:"repository"`
	Digest           string   `json:"digest"`
	AffectedTags     []string `json:"affectedTags"`
	RequiresReason   bool     `json:"requiresReason"`
	BlockedReasons   []string `json:"blockedReasons"`
	RelatedArtifacts []string `json:"relatedArtifacts"`
}

type ArtifactDeletion struct {
	ID           foundation.ID `json:"id"`
	RepositoryID foundation.ID `json:"repositoryId"`
	Repository   string        `json:"repository"`
	Digest       string        `json:"digest"`
	AffectedTags []string      `json:"affectedTags"`
	ActorID      foundation.ID `json:"actorId"`
	Reason       string        `json:"reason"`
	Status       string        `json:"status"`
	Message      string        `json:"message,omitempty"`
	StartedAt    time.Time     `json:"startedAt"`
	CompletedAt  *time.Time    `json:"completedAt,omitempty"`
}

type ManifestInventory struct {
	ID                       foundation.ID `json:"id"`
	RepositoryID             foundation.ID `json:"repositoryId"`
	Digest                   string        `json:"digest"`
	MediaType                string        `json:"mediaType,omitempty"`
	ArtifactType             string        `json:"artifactType,omitempty"`
	SubjectDigest            string        `json:"subjectDigest,omitempty"`
	ObservedKind             string        `json:"observedKind"`
	ArtifactRelationship     string        `json:"artifactRelationship"`
	ClassificationSource     string        `json:"classificationSource"`
	ClassificationConfidence string        `json:"classificationConfidence"`
	ManifestSize             int64         `json:"manifestSize"`
	Tags                     []string      `json:"tags"`
	State                    string        `json:"state"`
	FirstSeenAt              time.Time     `json:"firstSeenAt"`
	LastPushedAt             *time.Time    `json:"lastPushedAt,omitempty"`
	LastSeenAt               time.Time     `json:"lastSeenAt"`
	UntaggedAt               *time.Time    `json:"untaggedAt,omitempty"`
	DeletedAt                *time.Time    `json:"deletedAt,omitempty"`
}

type ManifestObservation struct {
	Digest                   string
	MediaType                string
	ArtifactType             string
	SubjectDigest            string
	ObservedKind             string
	ArtifactRelationship     string
	ClassificationSource     string
	ClassificationConfidence string
	ManifestSize             int64
	Tag                      string
	PushedAt                 *time.Time
}

type LifecyclePreview struct {
	ID               foundation.ID          `json:"id"`
	RepositoryID     foundation.ID          `json:"repositoryId"`
	Repository       string                 `json:"repository"`
	Status           string                 `json:"status"`
	InventoryAt      time.Time              `json:"inventoryAt"`
	CreatedBy        foundation.ID          `json:"createdBy"`
	CreatedAt        time.Time              `json:"createdAt"`
	ExpiresAt        time.Time              `json:"expiresAt"`
	EligibleCount    int                    `json:"eligibleCount"`
	RetainedCount    int                    `json:"retainedCount"`
	BlockedCount     int                    `json:"blockedCount"`
	PolicyVersion    int                    `json:"policyVersion"`
	EvaluatorVersion int                    `json:"evaluatorVersion"`
	Items            []LifecyclePreviewItem `json:"items"`
	PolicySnapshot   string                 `json:"-"`
}

type LifecyclePreviewItem struct {
	ID           foundation.ID `json:"id"`
	PreviewID    foundation.ID `json:"-"`
	ManifestID   foundation.ID `json:"-"`
	Digest       string        `json:"digest"`
	MediaType    string        `json:"mediaType,omitempty"`
	ArtifactType string        `json:"artifactType,omitempty"`
	Tags         []string      `json:"tags"`
	Decision     string        `json:"decision"`
	Reasons      []string      `json:"reasons"`
	CreatedAt    time.Time     `json:"-"`
}

type LifecycleRun struct {
	ID           foundation.ID      `json:"id"`
	PreviewID    foundation.ID      `json:"previewId"`
	RepositoryID foundation.ID      `json:"repositoryId"`
	Repository   string             `json:"repository"`
	ActorID      foundation.ID      `json:"actorId"`
	Reason       string             `json:"reason"`
	Status       string             `json:"status"`
	StartedAt    time.Time          `json:"startedAt"`
	CompletedAt  *time.Time         `json:"completedAt,omitempty"`
	Items        []LifecycleRunItem `json:"items"`
}

type LifecycleRunItem struct {
	ID        foundation.ID `json:"id"`
	RunID     foundation.ID `json:"-"`
	Digest    string        `json:"digest"`
	Status    string        `json:"status"`
	Message   string        `json:"message,omitempty"`
	CreatedAt time.Time     `json:"-"`
	UpdatedAt time.Time     `json:"-"`
}
