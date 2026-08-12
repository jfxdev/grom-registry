package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type ArtifactDeletionService struct {
	store        registrydomain.Store
	repositories *RepositoryService
	inventory    *InventoryService
	distribution DistributionMetadata
	audit        AuditRecorder
	now          func() time.Time
}

func NewArtifactDeletionService(
	store registrydomain.Store,
	repositories *RepositoryService,
	inventory *InventoryService,
	distribution DistributionMetadata,
	audit AuditRecorder,
) *ArtifactDeletionService {
	return &ArtifactDeletionService{
		store: store, repositories: repositories, inventory: inventory,
		distribution: distribution, audit: audit,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *ArtifactDeletionService) Preview(
	ctx context.Context,
	projectID foundation.ID,
	projectSlug, repository, reference, reason string,
	enforceReason bool,
) (*registrydomain.ArtifactDeletionPreview, error) {
	repository = strings.TrimSpace(repository)
	reference = strings.TrimSpace(reference)
	if !repositoryNamePattern.MatchString(repository) || len(repository) > 255 {
		return nil, fmt.Errorf("invalid repository path")
	}
	if reference == "" || len(reference) > 255 {
		return nil, fmt.Errorf("artifact reference is required")
	}
	if len(reason) > 500 {
		return nil, fmt.Errorf("deletion reason must not exceed 500 characters")
	}
	if _, err := s.store.FindRepository(ctx, projectID, repository); err != nil {
		return nil, err
	}

	fullRepository := projectSlug + "/" + repository
	inventory, err := s.inventory.Reconcile(ctx, projectID, projectSlug, repository)
	if err != nil {
		return nil, fmt.Errorf("inventory reconciliation failed: %w", err)
	}
	digest, exists, err := s.distribution.ResolveManifest(ctx, fullRepository, reference)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("artifact not found")
	}
	tags, err := s.distribution.ListRepositoryTags(ctx, fullRepository)
	if err != nil {
		return nil, err
	}
	affectedTags := make([]string, 0)
	for _, tag := range tags {
		tagDigest, tagExists, resolveErr := s.distribution.ResolveManifest(ctx, fullRepository, tag)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if tagExists && tagDigest == digest {
			affectedTags = append(affectedTags, tag)
		}
	}
	sort.Strings(affectedTags)
	requiresReason, err := s.repositories.EvaluateDeletion(
		ctx, projectID, repository, affectedTags, reason, enforceReason,
	)
	if err != nil {
		return nil, err
	}

	metadata, err := s.distribution.FetchManifest(ctx, fullRepository, digest)
	if err != nil {
		return nil, err
	}
	blockedReasons := make([]string, 0)
	related := make([]string, 0)
	if metadata.SubjectDigest != "" {
		blockedReasons = append(blockedReasons, "OCI referrer artifacts cannot be deleted manually")
		related = append(related, metadata.SubjectDigest)
	}
	referrers, err := s.distribution.ListReferrers(ctx, fullRepository, digest)
	if err != nil {
		return nil, err
	}
	if len(referrers) > 0 {
		blockedReasons = append(blockedReasons, "artifact has OCI referrers")
		for _, referrer := range referrers {
			related = append(related, referrer.Digest)
		}
	}
	sort.Strings(related)
	children := deletionChildDigests(digest, inventory)
	return &registrydomain.ArtifactDeletionPreview{
		Repository: repository, Digest: digest, AffectedTags: affectedTags,
		RequiresReason: requiresReason, BlockedReasons: blockedReasons,
		RelatedArtifacts: related,
		ChildDigests:     children,
	}, nil
}

func (s *ArtifactDeletionService) Execute(
	ctx context.Context,
	projectID foundation.ID,
	projectSlug, repository, reference, reason, expectedDigest string,
	expectedTags []string,
	expectedChildDigests []string,
	actor foundation.PrincipalRef,
) (*registrydomain.ArtifactDeletion, error) {
	reason = strings.TrimSpace(reason)
	preview, err := s.Preview(
		ctx, projectID, projectSlug, repository, reference, reason, true,
	)
	if err != nil {
		return nil, err
	}
	if expectedDigest == "" || expectedDigest != preview.Digest {
		return nil, fmt.Errorf("artifact digest changed; review the deletion again")
	}
	if !equalStrings(expectedTags, preview.AffectedTags) {
		return nil, fmt.Errorf("artifact tag aliases changed; review the deletion again")
	}
	if !equalStrings(expectedChildDigests, preview.ChildDigests) {
		return nil, fmt.Errorf("artifact child manifests changed; review the deletion again")
	}
	if len(preview.BlockedReasons) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(preview.BlockedReasons, "; "))
	}
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	now := s.now()
	deletion := &registrydomain.ArtifactDeletion{
		ID: foundation.NewID(), RepositoryID: target.ID, Repository: repository,
		Digest: preview.Digest, AffectedTags: preview.AffectedTags, ActorID: actor.ID,
		Reason: reason, Status: constants.ArtifactDeletionRunning, StartedAt: now,
	}
	if err := s.store.CreateArtifactDeletion(ctx, deletion); err != nil {
		return nil, err
	}
	if s.audit != nil {
		if err := s.audit.Record(ctx, actor, constants.AuditArtifactDeletionStarted,
			constants.AuditResourceArtifactDeletion, deletion.ID, map[string]any{
				"repository": repository, "digest": deletion.Digest,
				"affectedTags": deletion.AffectedTags, "reason": reason,
			}); err != nil {
			completedAt := s.now()
			_ = s.store.CompleteArtifactDeletion(
				ctx, deletion.ID, constants.ArtifactDeletionFailed,
				"audit recording failed before deletion", completedAt,
			)
			return nil, err
		}
	}

	fullRepository := projectSlug + "/" + repository
	deletedDigests, deleteErr := deleteManifestTree(
		ctx, s.distribution, fullRepository, deletion.Digest, preview.ChildDigests,
	)
	if deleteErr != nil {
		completedAt := s.now()
		deletion.Status = constants.ArtifactDeletionFailed
		deletion.Message = deleteErr.Error()
		deletion.CompletedAt = &completedAt
		if completeErr := s.store.CompleteArtifactDeletion(
			ctx, deletion.ID, deletion.Status, deletion.Message, completedAt,
		); completeErr != nil {
			return nil, completeErr
		}
		for _, deletedDigest := range deletedDigests {
			_ = s.store.MarkManifestDeleted(ctx, target.ID, deletedDigest, completedAt)
		}
		s.auditDeletionFailure(ctx, actor, deletion, deleteErr, len(deletedDigests) > 0)
		return deletion, deleteErr
	}

	completedAt := s.now()
	for _, deletedDigest := range deletedDigests {
		if err := s.store.MarkManifestDeleted(ctx, target.ID, deletedDigest, completedAt); err != nil {
			deletion.Status = constants.ArtifactDeletionFailed
			deletion.Message = "manifest deleted but inventory update failed"
			deletion.CompletedAt = &completedAt
			_ = s.store.CompleteArtifactDeletion(ctx, deletion.ID, deletion.Status, deletion.Message, completedAt)
			s.auditDeletionFailure(ctx, actor, deletion, err, true)
			return deletion, err
		}
	}
	deletion.Status = constants.ArtifactDeletionCompleted
	deletion.Message = fmt.Sprintf("manifest and %d unreferenced child manifests deleted; storage is reclaimed by a later garbage collection", len(preview.ChildDigests))
	deletion.CompletedAt = &completedAt
	if err := s.store.CompleteArtifactDeletion(
		ctx, deletion.ID, deletion.Status, deletion.Message, completedAt,
	); err != nil {
		s.auditDeletionFailure(ctx, actor, deletion, err, true)
		return deletion, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, actor, constants.AuditArtifactDeletionCompleted,
			constants.AuditResourceArtifactDeletion, deletion.ID, map[string]any{
				"repository": repository, "digest": deletion.Digest,
				"affectedTags": deletion.AffectedTags, "reason": reason,
				"childDigests": preview.ChildDigests,
			})
	}
	return deletion, nil
}

func (s *ArtifactDeletionService) auditDeletionFailure(
	ctx context.Context,
	actor foundation.PrincipalRef,
	deletion *registrydomain.ArtifactDeletion,
	cause error,
	manifestDeleted bool,
) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, actor, constants.AuditArtifactDeletionFailed,
		constants.AuditResourceArtifactDeletion, deletion.ID, map[string]any{
			"repository": deletion.Repository, "digest": deletion.Digest,
			"affectedTags": deletion.AffectedTags, "reason": deletion.Reason,
			"message": deletion.Message, "error": cause.Error(), "manifestDeleted": manifestDeleted,
		})
}

func (s *ArtifactDeletionService) List(
	ctx context.Context,
	projectID foundation.ID,
	repository string,
) ([]registrydomain.ArtifactDeletion, error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	deletions, err := s.store.ListArtifactDeletions(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	for i := range deletions {
		deletions[i].Repository = target.Name
	}
	return deletions, nil
}

func (s *ArtifactDeletionService) ListPage(ctx context.Context, projectID foundation.ID, repository string, request foundation.PageRequest) (foundation.PageResult[registrydomain.ArtifactDeletion], error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return foundation.PageResult[registrydomain.ArtifactDeletion]{}, err
	}
	result, err := s.store.ListArtifactDeletionsPage(ctx, target.ID, request)
	if err != nil {
		return foundation.PageResult[registrydomain.ArtifactDeletion]{}, err
	}
	for i := range result.Items {
		result.Items[i].Repository = target.Name
	}
	return result, nil
}
