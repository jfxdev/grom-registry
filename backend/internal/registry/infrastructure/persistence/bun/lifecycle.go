package bunstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func (s *Store) UpsertManifestObservation(
	ctx context.Context,
	repositoryID foundation.ID,
	observation registrydomain.ManifestObservation,
	observedAt time.Time,
) error {
	if observation.ObservedKind == "" {
		observation.ObservedKind = constants.ArtifactKindUnknownOCI
	}
	if observation.ArtifactRelationship == "" {
		observation.ArtifactRelationship = constants.ArtifactRelationshipPrimary
	}
	if observation.ClassificationSource == "" {
		observation.ClassificationSource = "unknown"
	}
	if observation.ClassificationConfidence == "" {
		observation.ClassificationConfidence = constants.ClassificationConfidenceNone
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		model := &manifestModel{
			ID: foundation.NewID().String(), RepositoryID: repositoryID.String(),
			Digest: observation.Digest, MediaType: observation.MediaType,
			ArtifactType: observation.ArtifactType, SubjectDigest: observation.SubjectDigest,
			ObservedKind: observation.ObservedKind, ArtifactRelationship: observation.ArtifactRelationship,
			ClassificationSource:     observation.ClassificationSource,
			ClassificationConfidence: observation.ClassificationConfidence,
			ManifestSize:             observation.ManifestSize, State: constants.InventoryStateUntagged,
			FirstSeenAt: observedAt, LastPushedAt: observation.PushedAt, LastSeenAt: observedAt,
			UntaggedAt: &observedAt,
		}
		manifestInsert := tx.NewInsert().Model(model).
			On("CONFLICT (repository_id, digest) DO UPDATE").
			Set("media_type = EXCLUDED.media_type").
			Set("artifact_type = EXCLUDED.artifact_type").
			Set("subject_digest = EXCLUDED.subject_digest").
			Set("observed_kind = EXCLUDED.observed_kind").
			Set("artifact_relationship = EXCLUDED.artifact_relationship").
			Set("classification_source = EXCLUDED.classification_source").
			Set("classification_confidence = EXCLUDED.classification_confidence").
			Set("manifest_size = EXCLUDED.manifest_size").
			Set("last_seen_at = EXCLUDED.last_seen_at").
			Set("deleted_at = NULL")
		if observation.PushedAt != nil {
			manifestInsert = manifestInsert.Set("last_pushed_at = EXCLUDED.last_pushed_at")
		}
		if err := manifestInsert.Returning("id").Scan(ctx, &model.ID); err != nil {
			return err
		}

		if observation.Tag != "" {
			var oldManifestID string
			tagErr := tx.NewSelect().Model((*tagModel)(nil)).
				Column("manifest_id").
				Where("repository_id = ?", repositoryID.String()).
				Where("name = ?", observation.Tag).
				Scan(ctx, &oldManifestID)
			if tagErr != nil && !errors.Is(tagErr, sql.ErrNoRows) {
				return tagErr
			}
			tag := &tagModel{
				RepositoryID: repositoryID.String(), Name: observation.Tag, ManifestID: model.ID,
				FirstSeenAt: observedAt, LastMovedAt: observedAt, LastSeenAt: observedAt,
			}
			if _, err := tx.NewInsert().Model(tag).
				ModelTableExpr("registry_tags AS current_tag").
				On("CONFLICT (repository_id, name) DO UPDATE").
				Set("manifest_id = EXCLUDED.manifest_id").
				Set(`last_moved_at = CASE
					WHEN current_tag.manifest_id <> EXCLUDED.manifest_id THEN EXCLUDED.last_moved_at
					ELSE current_tag.last_moved_at
				END`).
				Set("last_seen_at = EXCLUDED.last_seen_at").
				Set("detached_at = NULL").
				Exec(ctx); err != nil {
				return err
			}
			if oldManifestID != "" && oldManifestID != model.ID {
				if err := refreshManifestState(ctx, tx, oldManifestID, observedAt); err != nil {
					return err
				}
			}
			if _, err := tx.NewUpdate().Model((*manifestModel)(nil)).
				Set("state = ?", constants.InventoryStateActive).
				Set("untagged_at = NULL").
				Where("id = ?", model.ID).Exec(ctx); err != nil {
				return err
			}
		} else if err := refreshManifestState(ctx, tx, model.ID, observedAt); err != nil {
			return err
		}
		return nil
	})
}

func refreshManifestState(ctx context.Context, tx bun.Tx, manifestID string, now time.Time) error {
	active, err := tx.NewSelect().Model((*tagModel)(nil)).
		Where("manifest_id = ?", manifestID).
		Where("detached_at IS NULL").Exists(ctx)
	if err != nil {
		return err
	}
	if active {
		_, err = tx.NewUpdate().Model((*manifestModel)(nil)).
			Set("state = ?", constants.InventoryStateActive).
			Set("untagged_at = NULL").Where("id = ?", manifestID).Exec(ctx)
		return err
	}
	_, err = tx.NewUpdate().Model((*manifestModel)(nil)).
		Set("state = ?", constants.InventoryStateUntagged).
		Set("untagged_at = COALESCE(untagged_at, ?)", now).
		Where("id = ?", manifestID).
		Where("deleted_at IS NULL").Exec(ctx)
	return err
}

func (s *Store) CompleteInventoryReconciliation(
	ctx context.Context,
	repositoryID foundation.ID,
	seenTags, seenDigests []string,
	observedAt time.Time,
) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var tags []tagModel
		if err := tx.NewSelect().Model(&tags).
			Where("repository_id = ?", repositoryID.String()).
			Where("detached_at IS NULL").Scan(ctx); err != nil {
			return err
		}
		seenTagSet := stringSet(seenTags)
		affected := map[string]struct{}{}
		for i := range tags {
			if _, ok := seenTagSet[tags[i].Name]; ok {
				continue
			}
			tags[i].DetachedAt = &observedAt
			tags[i].LastSeenAt = observedAt
			affected[tags[i].ManifestID] = struct{}{}
			if _, err := tx.NewUpdate().Model(&tags[i]).WherePK().Exec(ctx); err != nil {
				return err
			}
		}
		for manifestID := range affected {
			if err := refreshManifestState(ctx, tx, manifestID, observedAt); err != nil {
				return err
			}
		}
		seenDigestSet := stringSet(seenDigests)
		var manifests []manifestModel
		if err := tx.NewSelect().Model(&manifests).
			Where("repository_id = ?", repositoryID.String()).
			Where("deleted_at IS NULL").Scan(ctx); err != nil {
			return err
		}
		for i := range manifests {
			if _, ok := seenDigestSet[manifests[i].Digest]; ok {
				continue
			}
			if manifests[i].State == constants.InventoryStateUntagged {
				continue
			}
			manifests[i].State = constants.InventoryStateMissing
			if _, err := tx.NewUpdate().Model(&manifests[i]).WherePK().Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func (s *Store) ListManifestInventory(ctx context.Context, repositoryID foundation.ID) ([]registrydomain.ManifestInventory, error) {
	var manifests []manifestModel
	if err := s.db.NewSelect().Model(&manifests).
		Where("repository_id = ?", repositoryID.String()).
		OrderExpr("last_seen_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]registrydomain.ManifestInventory, 0, len(manifests))
	for i := range manifests {
		var tags []tagModel
		if err := s.db.NewSelect().Model(&tags).
			Where("manifest_id = ?", manifests[i].ID).
			Where("detached_at IS NULL").OrderExpr("name ASC").Scan(ctx); err != nil {
			return nil, err
		}
		names := make([]string, len(tags))
		for j := range tags {
			names[j] = tags[j].Name
		}
		result = append(result, registrydomain.ManifestInventory{
			ID: foundation.ID(manifests[i].ID), RepositoryID: repositoryID,
			Digest: manifests[i].Digest, MediaType: manifests[i].MediaType,
			ArtifactType: manifests[i].ArtifactType, SubjectDigest: manifests[i].SubjectDigest,
			ObservedKind:             manifests[i].ObservedKind,
			ArtifactRelationship:     manifests[i].ArtifactRelationship,
			ClassificationSource:     manifests[i].ClassificationSource,
			ClassificationConfidence: manifests[i].ClassificationConfidence,
			ManifestSize:             manifests[i].ManifestSize, Tags: names, State: manifests[i].State,
			FirstSeenAt: manifests[i].FirstSeenAt, LastPushedAt: manifests[i].LastPushedAt,
			LastSeenAt: manifests[i].LastSeenAt, UntaggedAt: manifests[i].UntaggedAt,
			DeletedAt: manifests[i].DeletedAt,
		})
	}
	return result, nil
}

func (s *Store) ListManifestInventoryPage(ctx context.Context, repositoryID foundation.ID, request foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	boundary, err := keysetBoundary(request)
	if err != nil {
		return foundation.PageResult[registrydomain.ManifestInventory]{}, err
	}
	var models []manifestModel
	query := s.db.NewSelect().Model(&models).Where("repository_id = ?", repositoryID.String())
	if boundary != nil {
		query = query.Where("(first_seen_at < ? OR (first_seen_at = ? AND id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("first_seen_at DESC, id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[registrydomain.ManifestInventory]{}, err
	}
	more := len(models) > request.Limit
	if more {
		models = models[:request.Limit]
	}
	items, err := s.manifestInventoryFromModels(ctx, repositoryID, models)
	if err != nil {
		return foundation.PageResult[registrydomain.ManifestInventory]{}, err
	}
	result := foundation.PageResult[registrydomain.ManifestInventory]{Items: items}
	if more {
		result.NextCursor, err = encodeKeysetCursor(request.Scope, models[len(models)-1].FirstSeenAt, models[len(models)-1].ID)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Store) manifestInventoryFromModels(ctx context.Context, repositoryID foundation.ID, manifests []manifestModel) ([]registrydomain.ManifestInventory, error) {
	ids := make([]string, len(manifests))
	for i := range manifests {
		ids[i] = manifests[i].ID
	}
	tagsByManifest := make(map[string][]string, len(ids))
	if len(ids) > 0 {
		var tags []tagModel
		if err := s.db.NewSelect().Model(&tags).Where("manifest_id IN (?)", bun.List(ids)).Where("detached_at IS NULL").OrderExpr("name ASC").Scan(ctx); err != nil {
			return nil, err
		}
		for _, tag := range tags {
			tagsByManifest[tag.ManifestID] = append(tagsByManifest[tag.ManifestID], tag.Name)
		}
	}
	result := make([]registrydomain.ManifestInventory, 0, len(manifests))
	for i := range manifests {
		m := manifests[i]
		result = append(result, registrydomain.ManifestInventory{ID: foundation.ID(m.ID), RepositoryID: repositoryID, Digest: m.Digest, MediaType: m.MediaType, ArtifactType: m.ArtifactType, SubjectDigest: m.SubjectDigest, ObservedKind: m.ObservedKind, ArtifactRelationship: m.ArtifactRelationship, ClassificationSource: m.ClassificationSource, ClassificationConfidence: m.ClassificationConfidence, ManifestSize: m.ManifestSize, Tags: tagsByManifest[m.ID], State: m.State, FirstSeenAt: m.FirstSeenAt, LastPushedAt: m.LastPushedAt, LastSeenAt: m.LastSeenAt, UntaggedAt: m.UntaggedAt, DeletedAt: m.DeletedAt})
	}
	return result, nil
}

func (s *Store) MarkManifestDeleted(ctx context.Context, repositoryID foundation.ID, digest string, deletedAt time.Time) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var manifests []manifestModel
		if err := tx.NewSelect().Model(&manifests).
			Column("id").
			Where("repository_id = ?", repositoryID.String()).
			Where("digest = ?", digest).Scan(ctx); err != nil {
			return err
		}
		for i := range manifests {
			if _, err := tx.NewUpdate().Model((*tagModel)(nil)).
				Set("detached_at = ?", deletedAt).
				Where("manifest_id = ?", manifests[i].ID).
				Where("detached_at IS NULL").Exec(ctx); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model((*manifestModel)(nil)).
			Set("state = ?", constants.InventoryStateDeleted).
			Set("deleted_at = ?", deletedAt).
			Where("repository_id = ?", repositoryID.String()).
			Where("digest = ?", digest).Exec(ctx)
		return err
	})
}

func (s *Store) CreateArtifactDeletion(ctx context.Context, deletion *registrydomain.ArtifactDeletion) error {
	tags, err := json.Marshal(deletion.AffectedTags)
	if err != nil {
		return err
	}
	_, err = s.db.NewInsert().Model(&artifactDeletionModel{
		ID: deletion.ID.String(), RepositoryID: deletion.RepositoryID.String(),
		Digest: deletion.Digest, AffectedTags: string(tags), ActorID: deletion.ActorID.String(),
		Reason: deletion.Reason, Status: deletion.Status, Message: deletion.Message,
		StartedAt: deletion.StartedAt, CompletedAt: deletion.CompletedAt,
	}).Exec(ctx)
	return err
}

func (s *Store) CompleteArtifactDeletion(
	ctx context.Context,
	deletionID foundation.ID,
	status, message string,
	completedAt time.Time,
) error {
	_, err := s.db.NewUpdate().Model((*artifactDeletionModel)(nil)).
		Set("status = ?", status).
		Set("message = ?", message).
		Set("completed_at = ?", completedAt).
		Where("id = ?", deletionID.String()).Exec(ctx)
	return err
}

func (s *Store) ListArtifactDeletions(
	ctx context.Context,
	repositoryID foundation.ID,
) ([]registrydomain.ArtifactDeletion, error) {
	var models []artifactDeletionModel
	if err := s.db.NewSelect().Model(&models).
		Where("repository_id = ?", repositoryID.String()).
		OrderExpr("started_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]registrydomain.ArtifactDeletion, 0, len(models))
	for i := range models {
		var tags []string
		if err := json.Unmarshal([]byte(models[i].AffectedTags), &tags); err != nil {
			return nil, err
		}
		result = append(result, registrydomain.ArtifactDeletion{
			ID: foundation.ID(models[i].ID), RepositoryID: foundation.ID(models[i].RepositoryID),
			Digest: models[i].Digest, AffectedTags: tags, ActorID: foundation.ID(models[i].ActorID),
			Reason: models[i].Reason, Status: models[i].Status, Message: models[i].Message,
			StartedAt: models[i].StartedAt, CompletedAt: models[i].CompletedAt,
		})
	}
	return result, nil
}

func (s *Store) ListArtifactDeletionsPage(ctx context.Context, repositoryID foundation.ID, request foundation.PageRequest) (foundation.PageResult[registrydomain.ArtifactDeletion], error) {
	boundary, err := keysetBoundary(request)
	if err != nil {
		return foundation.PageResult[registrydomain.ArtifactDeletion]{}, err
	}
	var models []artifactDeletionModel
	query := s.db.NewSelect().Model(&models).Where("repository_id = ?", repositoryID.String())
	if boundary != nil {
		query = query.Where("(started_at < ? OR (started_at = ? AND id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("started_at DESC, id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[registrydomain.ArtifactDeletion]{}, err
	}
	more := len(models) > request.Limit
	if more {
		models = models[:request.Limit]
	}
	items := make([]registrydomain.ArtifactDeletion, 0, len(models))
	for _, model := range models {
		var tags []string
		if err := json.Unmarshal([]byte(model.AffectedTags), &tags); err != nil {
			return foundation.PageResult[registrydomain.ArtifactDeletion]{}, err
		}
		items = append(items, registrydomain.ArtifactDeletion{ID: foundation.ID(model.ID), RepositoryID: foundation.ID(model.RepositoryID), Digest: model.Digest, AffectedTags: tags, ActorID: foundation.ID(model.ActorID), Reason: model.Reason, Status: model.Status, Message: model.Message, StartedAt: model.StartedAt, CompletedAt: model.CompletedAt})
	}
	result := foundation.PageResult[registrydomain.ArtifactDeletion]{Items: items}
	if more {
		result.NextCursor, err = encodeKeysetCursor(request.Scope, models[len(models)-1].StartedAt, models[len(models)-1].ID)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Store) CreateLifecyclePreview(ctx context.Context, preview *registrydomain.LifecyclePreview) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		model := &lifecyclePreviewModel{
			ID: preview.ID.String(), RepositoryID: preview.RepositoryID.String(),
			PolicySnapshotJSON: preview.PolicySnapshot, Status: preview.Status,
			PolicyVersion: preview.PolicyVersion, EvaluatorVersion: preview.EvaluatorVersion,
			InventoryAt: preview.InventoryAt, CreatedBy: preview.CreatedBy.String(),
			CreatedAt: preview.CreatedAt, ExpiresAt: preview.ExpiresAt,
		}
		if _, err := tx.NewInsert().Model(model).Exec(ctx); err != nil {
			return err
		}
		for i := range preview.Items {
			reasons, _ := json.Marshal(preview.Items[i].Reasons)
			tags, _ := json.Marshal(preview.Items[i].Tags)
			item := &lifecyclePreviewItemModel{
				ID: preview.Items[i].ID.String(), PreviewID: preview.ID.String(),
				ManifestID: preview.Items[i].ManifestID.String(), Digest: preview.Items[i].Digest,
				MediaType: preview.Items[i].MediaType, ArtifactType: preview.Items[i].ArtifactType,
				Decision: preview.Items[i].Decision, ReasonsJSON: string(reasons),
				ExpectedTagsJSON: string(tags), CreatedAt: preview.Items[i].CreatedAt,
			}
			if _, err := tx.NewInsert().Model(item).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) FindLifecyclePreview(ctx context.Context, previewID foundation.ID) (*registrydomain.LifecyclePreview, error) {
	model := new(lifecyclePreviewModel)
	if err := s.db.NewSelect().Model(model).Where("id = ?", previewID.String()).Scan(ctx); err != nil {
		return nil, err
	}
	var items []lifecyclePreviewItemModel
	if err := s.db.NewSelect().Model(&items).Where("preview_id = ?", model.ID).
		OrderExpr("digest ASC").Scan(ctx); err != nil {
		return nil, err
	}
	preview := &registrydomain.LifecyclePreview{
		ID: foundation.ID(model.ID), RepositoryID: foundation.ID(model.RepositoryID),
		Status: model.Status, InventoryAt: model.InventoryAt, CreatedBy: foundation.ID(model.CreatedBy),
		PolicyVersion: model.PolicyVersion, EvaluatorVersion: model.EvaluatorVersion,
		CreatedAt: model.CreatedAt, ExpiresAt: model.ExpiresAt, PolicySnapshot: model.PolicySnapshotJSON,
	}
	for i := range items {
		var reasons, tags []string
		_ = json.Unmarshal([]byte(items[i].ReasonsJSON), &reasons)
		_ = json.Unmarshal([]byte(items[i].ExpectedTagsJSON), &tags)
		item := registrydomain.LifecyclePreviewItem{
			ID: foundation.ID(items[i].ID), PreviewID: preview.ID,
			ManifestID: foundation.ID(items[i].ManifestID), Digest: items[i].Digest,
			MediaType: items[i].MediaType, ArtifactType: items[i].ArtifactType,
			Decision: items[i].Decision, Reasons: reasons, Tags: tags, CreatedAt: items[i].CreatedAt,
		}
		preview.Items = append(preview.Items, item)
		countDecision(preview, item.Decision)
	}
	return preview, nil
}

func countDecision(preview *registrydomain.LifecyclePreview, decision string) {
	switch decision {
	case constants.LifecycleDecisionEligible:
		preview.EligibleCount++
	case constants.LifecycleDecisionBlocked:
		preview.BlockedCount++
	default:
		preview.RetainedCount++
	}
}

func (s *Store) SetLifecyclePreviewStatus(ctx context.Context, previewID foundation.ID, status string) error {
	_, err := s.db.NewUpdate().Model((*lifecyclePreviewModel)(nil)).
		Set("status = ?", status).Where("id = ?", previewID.String()).Exec(ctx)
	return err
}

func (s *Store) AcquireLifecycleLock(ctx context.Context, repositoryID, runID foundation.ID, now time.Time) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var existing lifecycleLockModel
		err := tx.NewSelect().Model(&existing).
			Where("repository_id = ?", repositoryID.String()).Scan(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && existing.AcquiredAt.Before(now.Add(-30*time.Minute)) {
			var interrupted lifecycleRunModel
			runErr := tx.NewSelect().Model(&interrupted).
				Where("id = ?", existing.RunID).Scan(ctx)
			if runErr != nil && !errors.Is(runErr, sql.ErrNoRows) {
				return runErr
			}
			if runErr == nil {
				if _, err := tx.NewUpdate().Model((*lifecycleRunModel)(nil)).
					Set("status = ?", constants.LifecycleRunFailed).
					Set("completed_at = ?", now).
					Where("id = ?", interrupted.ID).
					Where("status = ?", constants.LifecycleRunRunning).Exec(ctx); err != nil {
					return err
				}
				if _, err := tx.NewUpdate().Model((*lifecyclePreviewModel)(nil)).
					Set("status = ?", constants.LifecyclePreviewExecuted).
					Where("id = ?", interrupted.PreviewID).
					Where("status = ?", constants.LifecyclePreviewExecuting).Exec(ctx); err != nil {
					return err
				}
			}
			if _, err := tx.NewDelete().Model((*lifecycleLockModel)(nil)).
				Where("repository_id = ?", repositoryID.String()).
				Where("run_id = ?", existing.RunID).Exec(ctx); err != nil {
				return err
			}
		}
		_, err = tx.NewInsert().Model(&lifecycleLockModel{
			RepositoryID: repositoryID.String(), RunID: runID.String(), AcquiredAt: now,
		}).Exec(ctx)
		if err != nil {
			return fmt.Errorf("another lifecycle execution is already running")
		}
		return nil
	})
}

func (s *Store) ReleaseLifecycleLock(ctx context.Context, repositoryID, runID foundation.ID) error {
	_, err := s.db.NewDelete().Model((*lifecycleLockModel)(nil)).
		Where("repository_id = ?", repositoryID.String()).
		Where("run_id = ?", runID.String()).Exec(ctx)
	return err
}

func (s *Store) CreateLifecycleRunFromPreview(ctx context.Context, run *registrydomain.LifecycleRun) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewUpdate().Model((*lifecyclePreviewModel)(nil)).
			Set("status = ?", constants.LifecyclePreviewExecuting).
			Where("id = ?", run.PreviewID.String()).
			Where("status = ?", constants.LifecyclePreviewReady).Exec(ctx)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return fmt.Errorf("lifecycle preview is expired, executing, or already used")
		}
		model := &lifecycleRunModel{
			ID: run.ID.String(), PreviewID: run.PreviewID.String(), RepositoryID: run.RepositoryID.String(),
			ActorID: run.ActorID.String(), Reason: run.Reason, Status: run.Status, StartedAt: run.StartedAt,
		}
		if _, err := tx.NewInsert().Model(model).Exec(ctx); err != nil {
			return err
		}
		for i := range run.Items {
			item := lifecycleRunItemModel{
				ID: run.Items[i].ID.String(), RunID: run.ID.String(), Digest: run.Items[i].Digest,
				Status: run.Items[i].Status, Message: run.Items[i].Message,
				CreatedAt: run.Items[i].CreatedAt, UpdatedAt: run.Items[i].UpdatedAt,
			}
			if _, err := tx.NewInsert().Model(&item).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) UpdateLifecycleRunItem(ctx context.Context, item *registrydomain.LifecycleRunItem) error {
	_, err := s.db.NewUpdate().Model((*lifecycleRunItemModel)(nil)).
		Set("status = ?", item.Status).Set("message = ?", item.Message).
		Set("updated_at = ?", item.UpdatedAt).Where("id = ?", item.ID.String()).Exec(ctx)
	return err
}

func (s *Store) CompleteLifecycleRun(ctx context.Context, runID foundation.ID, status string, completedAt time.Time) error {
	_, err := s.db.NewUpdate().Model((*lifecycleRunModel)(nil)).
		Set("status = ?", status).Set("completed_at = ?", completedAt).
		Where("id = ?", runID.String()).Exec(ctx)
	return err
}

func (s *Store) ListLifecycleRuns(ctx context.Context, repositoryID foundation.ID) ([]registrydomain.LifecycleRun, error) {
	var models []lifecycleRunModel
	if err := s.db.NewSelect().Model(&models).Where("repository_id = ?", repositoryID.String()).
		OrderExpr("started_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]registrydomain.LifecycleRun, 0, len(models))
	for i := range models {
		run, err := s.findLifecycleRunFromModel(ctx, &models[i])
		if err != nil {
			return nil, err
		}
		result = append(result, *run)
	}
	return result, nil
}

func (s *Store) ListLifecycleRunsPage(ctx context.Context, repositoryID foundation.ID, request foundation.PageRequest) (foundation.PageResult[registrydomain.LifecycleRun], error) {
	boundary, err := keysetBoundary(request)
	if err != nil {
		return foundation.PageResult[registrydomain.LifecycleRun]{}, err
	}
	var models []lifecycleRunModel
	query := s.db.NewSelect().Model(&models).Where("repository_id = ?", repositoryID.String())
	if boundary != nil {
		query = query.Where("(started_at < ? OR (started_at = ? AND id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("started_at DESC, id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[registrydomain.LifecycleRun]{}, err
	}
	more := len(models) > request.Limit
	if more {
		models = models[:request.Limit]
	}
	items := make([]registrydomain.LifecycleRun, 0, len(models))
	for i := range models {
		run, err := s.findLifecycleRunFromModel(ctx, &models[i])
		if err != nil {
			return foundation.PageResult[registrydomain.LifecycleRun]{}, err
		}
		items = append(items, *run)
	}
	result := foundation.PageResult[registrydomain.LifecycleRun]{Items: items}
	if more {
		result.NextCursor, err = encodeKeysetCursor(request.Scope, models[len(models)-1].StartedAt, models[len(models)-1].ID)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

type keysetCursor struct {
	at time.Time
	id string
}

func keysetBoundary(request foundation.PageRequest) (*keysetCursor, error) {
	if request.Cursor == "" {
		return nil, nil
	}
	cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
	if err != nil || cursor.Timestamp == "" || cursor.ID == "" {
		return nil, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, cursor.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &keysetCursor{at: at, id: cursor.ID}, nil
}

func encodeKeysetCursor(scope string, at time.Time, id string) (string, error) {
	return foundation.EncodePageCursor(foundation.PageCursor{Scope: scope, Timestamp: at.UTC().Format(time.RFC3339Nano), ID: id})
}

func (s *Store) FindLifecycleRun(ctx context.Context, runID foundation.ID) (*registrydomain.LifecycleRun, error) {
	model := new(lifecycleRunModel)
	if err := s.db.NewSelect().Model(model).Where("id = ?", runID.String()).Scan(ctx); err != nil {
		return nil, err
	}
	return s.findLifecycleRunFromModel(ctx, model)
}

func (s *Store) findLifecycleRunFromModel(ctx context.Context, model *lifecycleRunModel) (*registrydomain.LifecycleRun, error) {
	var items []lifecycleRunItemModel
	if err := s.db.NewSelect().Model(&items).Where("run_id = ?", model.ID).
		OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	run := &registrydomain.LifecycleRun{
		ID: foundation.ID(model.ID), PreviewID: foundation.ID(model.PreviewID),
		RepositoryID: foundation.ID(model.RepositoryID), ActorID: foundation.ID(model.ActorID),
		Reason: model.Reason, Status: model.Status, StartedAt: model.StartedAt, CompletedAt: model.CompletedAt,
	}
	for i := range items {
		run.Items = append(run.Items, registrydomain.LifecycleRunItem{
			ID: foundation.ID(items[i].ID), RunID: run.ID, Digest: items[i].Digest,
			Status: items[i].Status, Message: items[i].Message,
			CreatedAt: items[i].CreatedAt, UpdatedAt: items[i].UpdatedAt,
		})
	}
	sort.Slice(run.Items, func(i, j int) bool { return run.Items[i].Digest < run.Items[j].Digest })
	return run, nil
}
