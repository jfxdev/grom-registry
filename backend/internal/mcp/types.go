package mcp

import (
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

// The MCP endpoint returns its own transfer shapes instead of domain structs
// so that fields outside its scope, such as repository policies, can never
// leak through a domain change.

type StorageUsage struct {
	Status         string     `json:"status" jsonschema:"pending, reconciled or unavailable"`
	AccountedBytes *int64     `json:"accountedBytes,omitempty" jsonschema:"logical OCI descriptor bytes, deduplicated by digest inside the scope"`
	ReconciledAt   *time.Time `json:"reconciledAt,omitempty"`
}

type Project struct {
	Slug           string       `json:"slug" jsonschema:"first path segment of every repository in the project"`
	Name           string       `json:"name"`
	Mode           string       `json:"mode" jsonschema:"standard or a pull-through mode"`
	Upstream       string       `json:"upstream,omitempty"`
	CreatedAt      time.Time    `json:"createdAt"`
	AccountedUsage StorageUsage `json:"accountedUsage"`
}

type ProjectDetail struct {
	Project
	CanManage bool `json:"canManage" jsonschema:"whether this credential has project Admin membership"`
}

type Repository struct {
	Name               string       `json:"name" jsonschema:"repository name inside the project, without the project slug"`
	Description        string       `json:"description"`
	Status             string       `json:"status"`
	CreationSource     string       `json:"creationSource"`
	Profile            string       `json:"profile"`
	ProfileSource      string       `json:"profileSource"`
	ProfileConfidence  string       `json:"profileConfidence"`
	ProfileNeedsReview bool         `json:"profileNeedsReview"`
	ProfileInferredAt  *time.Time   `json:"profileInferredAt,omitempty"`
	AccountedUsage     StorageUsage `json:"accountedUsage"`
	CreatedAt          time.Time    `json:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
}

type Platform struct {
	OS             string `json:"os,omitempty"`
	Architecture   string `json:"architecture,omitempty"`
	Variant        string `json:"variant,omitempty"`
	Digest         string `json:"digest,omitempty"`
	CompressedSize int64  `json:"compressedSize"`
}

type Artifact struct {
	Digest                   string     `json:"digest"`
	MediaType                string     `json:"mediaType,omitempty"`
	ArtifactType             string     `json:"artifactType,omitempty"`
	SubjectDigest            string     `json:"subjectDigest,omitempty" jsonschema:"digest this artifact refers to when it is an OCI referrer"`
	ObservedKind             string     `json:"observedKind"`
	ArtifactRelationship     string     `json:"artifactRelationship" jsonschema:"primary or referrer"`
	ClassificationSource     string     `json:"classificationSource"`
	ClassificationConfidence string     `json:"classificationConfidence"`
	ManifestSize             int64      `json:"manifestSize"`
	Platforms                []Platform `json:"platforms"`
	Tags                     []string   `json:"tags"`
	State                    string     `json:"state"`
	FirstSeenAt              time.Time  `json:"firstSeenAt"`
	LastPushedAt             *time.Time `json:"lastPushedAt,omitempty"`
	LastSeenAt               time.Time  `json:"lastSeenAt"`
	UntaggedAt               *time.Time `json:"untaggedAt,omitempty"`
	DeletedAt                *time.Time `json:"deletedAt,omitempty"`
}

type PresetPolicy struct {
	Type                 string   `json:"type"`
	TagPatterns          []string `json:"tagPatterns,omitempty"`
	PreventOverwrite     bool     `json:"preventOverwrite"`
	PreventDeletion      bool     `json:"preventDeletion"`
	ExcludeFromLifecycle bool     `json:"excludeFromLifecycle"`
	ExpireAfterDays      *int     `json:"expireAfterDays,omitempty"`
	KeepLast             *int     `json:"keepLast,omitempty"`
	UntaggedGraceDays    *int     `json:"untaggedGraceDays,omitempty"`
	AllowedPatterns      []string `json:"allowedPatterns,omitempty"`
	RequireReason        bool     `json:"requireReason"`
}

type PolicyPreset struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Description string       `json:"description"`
	Outcome     string       `json:"outcome"`
	Caution     string       `json:"caution,omitempty"`
	Policy      PresetPolicy `json:"policy"`
}

type LifecycleRunItem struct {
	Digest  string `json:"digest"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type LifecycleRun struct {
	ID          string             `json:"id"`
	Repository  string             `json:"repository,omitempty"`
	ActorID     string             `json:"actorId"`
	Reason      string             `json:"reason,omitempty"`
	Status      string             `json:"status"`
	StartedAt   time.Time          `json:"startedAt"`
	CompletedAt *time.Time         `json:"completedAt,omitempty"`
	Items       []LifecycleRunItem `json:"items"`
}

type ArtifactDeletion struct {
	ID           string     `json:"id"`
	Digest       string     `json:"digest"`
	AffectedTags []string   `json:"affectedTags"`
	ActorID      string     `json:"actorId"`
	Reason       string     `json:"reason,omitempty"`
	Status       string     `json:"status"`
	Message      string     `json:"message,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

type ListResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty" jsonschema:"pass as cursor with the same arguments to read the next page"`
}

func usageDTO(usage foundation.AccountedStorageUsage) StorageUsage {
	return StorageUsage{Status: usage.Status, AccountedBytes: usage.AccountedBytes, ReconciledAt: usage.ReconciledAt}
}

func projectDTO(project projectdomain.Project) Project {
	return Project{
		Slug: project.Slug, Name: project.Name, Mode: project.Mode, Upstream: project.Upstream,
		CreatedAt: project.CreatedAt, AccountedUsage: usageDTO(project.AccountedUsage),
	}
}

func repositoryDTO(repository registrydomain.Repository) Repository {
	return Repository{
		Name: repository.Name, Description: repository.Description, Status: repository.Status,
		CreationSource: repository.CreationSource, Profile: repository.Profile,
		ProfileSource: repository.ProfileSource, ProfileConfidence: repository.ProfileConfidence,
		ProfileNeedsReview: repository.ProfileNeedsReview, ProfileInferredAt: repository.ProfileInferredAt,
		AccountedUsage: usageDTO(repository.AccountedUsage),
		CreatedAt:      repository.CreatedAt, UpdatedAt: repository.UpdatedAt,
	}
}

func artifactDTO(item registrydomain.ManifestInventory) Artifact {
	platforms := make([]Platform, len(item.Platforms))
	for i, platform := range item.Platforms {
		platforms[i] = Platform{
			OS: platform.OS, Architecture: platform.Architecture, Variant: platform.Variant,
			Digest: platform.Digest, CompressedSize: platform.CompressedSize,
		}
	}
	return Artifact{
		Digest: item.Digest, MediaType: item.MediaType, ArtifactType: item.ArtifactType,
		SubjectDigest: item.SubjectDigest, ObservedKind: item.ObservedKind,
		ArtifactRelationship: item.ArtifactRelationship, ClassificationSource: item.ClassificationSource,
		ClassificationConfidence: item.ClassificationConfidence, ManifestSize: item.ManifestSize,
		Platforms: platforms, Tags: nonNil(item.Tags), State: item.State,
		FirstSeenAt: item.FirstSeenAt, LastPushedAt: item.LastPushedAt, LastSeenAt: item.LastSeenAt,
		UntaggedAt: item.UntaggedAt, DeletedAt: item.DeletedAt,
	}
}

func presetDTO(preset registrydomain.PolicyPreset) PolicyPreset {
	policy := preset.Policy
	return PolicyPreset{
		Key: preset.Key, Name: preset.Name, Category: preset.Category, Description: preset.Description,
		Outcome: preset.Outcome, Caution: preset.Caution,
		Policy: PresetPolicy{
			Type: policy.Type, TagPatterns: policy.TagPatterns, PreventOverwrite: policy.PreventOverwrite,
			PreventDeletion: policy.PreventDeletion, ExcludeFromLifecycle: policy.ExcludeFromLifecycle,
			ExpireAfterDays: policy.ExpireAfterDays, KeepLast: policy.KeepLast,
			UntaggedGraceDays: policy.UntaggedGraceDays, AllowedPatterns: policy.AllowedPatterns,
			RequireReason: policy.RequireReason,
		},
	}
}

func lifecycleRunDTO(run registrydomain.LifecycleRun) LifecycleRun {
	items := make([]LifecycleRunItem, len(run.Items))
	for i, item := range run.Items {
		items[i] = LifecycleRunItem{Digest: item.Digest, Status: item.Status, Message: item.Message}
	}
	return LifecycleRun{
		ID: run.ID.String(), Repository: run.Repository, ActorID: run.ActorID.String(), Reason: run.Reason,
		Status: run.Status, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Items: items,
	}
}

func artifactDeletionDTO(deletion registrydomain.ArtifactDeletion) ArtifactDeletion {
	return ArtifactDeletion{
		ID: deletion.ID.String(), Digest: deletion.Digest, AffectedTags: nonNil(deletion.AffectedTags),
		ActorID: deletion.ActorID.String(), Reason: deletion.Reason, Status: deletion.Status,
		Message: deletion.Message, StartedAt: deletion.StartedAt, CompletedAt: deletion.CompletedAt,
	}
}

func mapItems[T, R any](items []T, mapper func(T) R) []R {
	result := make([]R, len(items))
	for i, item := range items {
		result[i] = mapper(item)
	}
	return result
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
