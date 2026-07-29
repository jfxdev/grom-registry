package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type AuditRecorder interface {
	Record(
		ctx context.Context,
		actor foundation.PrincipalRef,
		action, resourceKind string,
		resourceID foundation.ID,
		metadata map[string]any,
	) error
}

type LifecycleService struct {
	store        registrydomain.Store
	inventory    *InventoryService
	distribution DistributionMetadata
	audit        AuditRecorder
	now          func() time.Time
	previewTTL   time.Duration
}

func NewLifecycleService(
	store registrydomain.Store,
	inventory *InventoryService,
	distribution DistributionMetadata,
	audit AuditRecorder,
) *LifecycleService {
	return &LifecycleService{
		store: store, inventory: inventory, distribution: distribution, audit: audit,
		now: func() time.Time { return time.Now().UTC() }, previewTTL: 15 * time.Minute,
	}
}

func (s *LifecycleService) CreatePreview(
	ctx context.Context,
	projectID foundation.ID,
	projectSlug, repository string,
	actor foundation.PrincipalRef,
) (*registrydomain.LifecyclePreview, error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	inventory, err := s.inventory.Reconcile(ctx, projectID, projectSlug, repository)
	if err != nil {
		return nil, fmt.Errorf("inventory reconciliation failed: %w", err)
	}
	now := s.now()
	items, err := evaluateLifecycle(target.Policies, inventory, now)
	if err != nil {
		return nil, err
	}
	snapshot, err := json.Marshal(target.Policies)
	if err != nil {
		return nil, err
	}
	preview := &registrydomain.LifecyclePreview{
		ID: foundation.NewID(), RepositoryID: target.ID, Repository: repository,
		Status: constants.LifecyclePreviewReady, InventoryAt: now,
		CreatedBy: actor.ID, CreatedAt: now, ExpiresAt: now.Add(s.previewTTL),
		PolicyVersion: target.PolicyVersion, EvaluatorVersion: constants.LifecycleEvaluatorVersion,
		Items: items, PolicySnapshot: string(snapshot),
	}
	for i := range preview.Items {
		preview.Items[i].ID = foundation.NewID()
		preview.Items[i].PreviewID = preview.ID
		preview.Items[i].CreatedAt = now
		countLifecycleDecision(preview, preview.Items[i].Decision)
	}
	if err := s.store.CreateLifecyclePreview(ctx, preview); err != nil {
		return nil, err
	}
	if s.audit != nil {
		if err := s.audit.Record(ctx, actor, constants.AuditLifecyclePreviewCreated,
			constants.AuditResourceRepository, target.ID, map[string]any{
				"previewId": preview.ID, "repository": repository,
				"eligible": preview.EligibleCount, "retained": preview.RetainedCount,
				"blocked": preview.BlockedCount,
			}); err != nil {
			return nil, err
		}
	}
	return preview, nil
}

func evaluateLifecycle(
	policies []registrydomain.Policy,
	inventory []registrydomain.ManifestInventory,
	now time.Time,
) ([]registrydomain.LifecyclePreviewItem, error) {
	retention := make([]registrydomain.Policy, 0)
	for _, policy := range policies {
		if policy.Enabled && policy.Type == constants.RepositoryPolicyRetention {
			retention = append(retention, policy)
		}
	}
	if len(retention) == 0 {
		return nil, fmt.Errorf("repository has no enabled retention policy")
	}

	protected := map[string][]string{}
	for _, manifest := range inventory {
		for _, policy := range policies {
			if !policy.Enabled || (!policy.ExcludeFromLifecycle &&
				(policy.Type != constants.RepositoryPolicyTagProtection || !policy.PreventDeletion)) {
				continue
			}
			for _, tag := range manifest.Tags {
				if matchesAnyTag(tag, policy.TagPatterns) {
					protected[manifest.Digest] = append(protected[manifest.Digest], fmt.Sprintf("tag %q is excluded from lifecycle", tag))
				}
			}
		}
	}

	referrerSubjects := map[string]struct{}{}
	for _, manifest := range inventory {
		if manifest.SubjectDigest != "" && manifest.State != constants.InventoryStateDeleted {
			referrerSubjects[manifest.SubjectDigest] = struct{}{}
		}
	}

	eligibleReasons := map[string][]string{}
	keepReasons := map[string][]string{}
	for _, policy := range retention {
		matching := make([]registrydomain.ManifestInventory, 0)
		for _, manifest := range inventory {
			if manifest.State == constants.InventoryStateDeleted || manifest.State == constants.InventoryStateMissing || len(manifest.Tags) == 0 {
				continue
			}
			for _, tag := range manifest.Tags {
				if matchesAnyTag(tag, policy.TagPatterns) {
					matching = append(matching, manifest)
					break
				}
			}
		}
		sort.SliceStable(matching, func(i, j int) bool {
			return lifecycleTime(matching[i]).After(lifecycleTime(matching[j]))
		})
		seen := map[string]struct{}{}
		distinct := make([]registrydomain.ManifestInventory, 0, len(matching))
		for _, manifest := range matching {
			if _, ok := seen[manifest.Digest]; ok {
				continue
			}
			seen[manifest.Digest] = struct{}{}
			distinct = append(distinct, manifest)
		}
		for index, manifest := range distinct {
			if policy.KeepLast != nil && index < *policy.KeepLast {
				keepReasons[manifest.Digest] = append(keepReasons[manifest.Digest], fmt.Sprintf("kept among the latest %d artifacts", *policy.KeepLast))
				continue
			}
			if policy.ExpireAfterDays != nil {
				threshold := now.Add(-time.Duration(*policy.ExpireAfterDays) * 24 * time.Hour)
				if lifecycleTime(manifest).After(threshold) {
					keepReasons[manifest.Digest] = append(keepReasons[manifest.Digest], fmt.Sprintf("younger than %d days", *policy.ExpireAfterDays))
					continue
				}
				eligibleReasons[manifest.Digest] = append(eligibleReasons[manifest.Digest], fmt.Sprintf("older than %d days", *policy.ExpireAfterDays))
			} else {
				eligibleReasons[manifest.Digest] = append(eligibleReasons[manifest.Digest], "outside the retained latest artifacts")
			}
		}
		for _, manifest := range inventory {
			if manifest.State != constants.InventoryStateUntagged || manifest.UntaggedAt == nil || policy.UntaggedGraceDays == nil {
				continue
			}
			threshold := now.Add(-time.Duration(*policy.UntaggedGraceDays) * 24 * time.Hour)
			if !manifest.UntaggedAt.After(threshold) {
				eligibleReasons[manifest.Digest] = append(eligibleReasons[manifest.Digest],
					fmt.Sprintf("untagged for at least %d days", *policy.UntaggedGraceDays))
			} else {
				keepReasons[manifest.Digest] = append(keepReasons[manifest.Digest],
					fmt.Sprintf("inside the %d day untagged grace period", *policy.UntaggedGraceDays))
			}
		}
	}

	items := make([]registrydomain.LifecyclePreviewItem, 0, len(inventory))
	for _, manifest := range inventory {
		if manifest.State == constants.InventoryStateDeleted || manifest.State == constants.InventoryStateMissing {
			continue
		}
		decision := constants.LifecycleDecisionRetained
		reasons := keepReasons[manifest.Digest]
		if eligible := eligibleReasons[manifest.Digest]; len(eligible) > 0 && len(keepReasons[manifest.Digest]) == 0 {
			decision = constants.LifecycleDecisionEligible
			reasons = eligible
		}
		if blocked := protected[manifest.Digest]; len(blocked) > 0 {
			decision = constants.LifecycleDecisionBlocked
			reasons = blocked
		}
		if manifest.SubjectDigest != "" {
			decision = constants.LifecycleDecisionBlocked
			reasons = []string{"OCI referrer artifacts are protected from lifecycle"}
		}
		if _, hasReferrers := referrerSubjects[manifest.Digest]; hasReferrers {
			decision = constants.LifecycleDecisionBlocked
			reasons = []string{"artifact has OCI referrers"}
		}
		if len(reasons) == 0 {
			reasons = []string{"not selected by the active retention policy"}
		}
		items = append(items, registrydomain.LifecyclePreviewItem{
			ManifestID: manifest.ID, Digest: manifest.Digest, MediaType: manifest.MediaType,
			ArtifactType: manifest.ArtifactType, Tags: append([]string(nil), manifest.Tags...),
			Decision: decision, Reasons: reasons,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Digest < items[j].Digest })
	return items, nil
}

func lifecycleTime(manifest registrydomain.ManifestInventory) time.Time {
	if manifest.LastPushedAt != nil {
		return *manifest.LastPushedAt
	}
	return manifest.FirstSeenAt
}

func countLifecycleDecision(preview *registrydomain.LifecyclePreview, decision string) {
	switch decision {
	case constants.LifecycleDecisionEligible:
		preview.EligibleCount++
	case constants.LifecycleDecisionBlocked:
		preview.BlockedCount++
	default:
		preview.RetainedCount++
	}
}

func (s *LifecycleService) Execute(
	ctx context.Context,
	projectID foundation.ID,
	projectSlug string,
	previewID foundation.ID,
	reason string,
	actor foundation.PrincipalRef,
) (*registrydomain.LifecycleRun, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 500 {
		return nil, fmt.Errorf("an execution reason between 1 and 500 characters is required")
	}
	preview, err := s.store.FindLifecyclePreview(ctx, previewID)
	if err != nil {
		return nil, err
	}
	if preview.Status == constants.LifecyclePreviewReady && !s.now().Before(preview.ExpiresAt) {
		_ = s.store.SetLifecyclePreviewStatus(ctx, preview.ID, constants.LifecyclePreviewExpired)
		preview.Status = constants.LifecyclePreviewExpired
	}
	if preview.Status != constants.LifecyclePreviewReady {
		return nil, fmt.Errorf("lifecycle preview is expired or already used")
	}
	target, err := s.store.FindRepositoryByID(ctx, preview.RepositoryID)
	if err != nil {
		return nil, err
	}
	if target.ProjectID != projectID {
		return nil, fmt.Errorf("lifecycle preview does not belong to this project")
	}
	run := &registrydomain.LifecycleRun{
		ID: foundation.NewID(), PreviewID: preview.ID, RepositoryID: target.ID,
		Repository: target.Name, ActorID: actor.ID, Reason: reason,
		Status: constants.LifecycleRunRunning, StartedAt: s.now(),
	}
	for _, item := range preview.Items {
		if item.Decision != constants.LifecycleDecisionEligible {
			continue
		}
		run.Items = append(run.Items, registrydomain.LifecycleRunItem{
			ID: foundation.NewID(), RunID: run.ID, Digest: item.Digest,
			Status: constants.LifecycleItemSkipped, Message: "waiting for revalidation",
			CreatedAt: run.StartedAt, UpdatedAt: run.StartedAt,
		})
	}
	if len(run.Items) == 0 {
		return nil, fmt.Errorf("lifecycle preview has no eligible artifacts")
	}
	if err := s.store.AcquireLifecycleLock(ctx, target.ID, run.ID, run.StartedAt); err != nil {
		return nil, err
	}
	defer func() { _ = s.store.ReleaseLifecycleLock(context.Background(), target.ID, run.ID) }()
	if err := s.store.CreateLifecycleRunFromPreview(ctx, run); err != nil {
		return nil, err
	}
	if s.audit != nil {
		if err := s.audit.Record(ctx, actor, constants.AuditLifecycleRunStarted,
			constants.AuditResourceLifecycleRun, run.ID, map[string]any{
				"previewId": preview.ID, "repository": target.Name, "reason": reason,
			}); err != nil {
			completedAt := s.now()
			_ = s.store.CompleteLifecycleRun(ctx, run.ID, constants.LifecycleRunFailed, completedAt)
			_ = s.store.SetLifecyclePreviewStatus(ctx, preview.ID, constants.LifecyclePreviewExecuted)
			return nil, err
		}
	}

	previewItems := map[string]registrydomain.LifecyclePreviewItem{}
	for _, item := range preview.Items {
		previewItems[item.Digest] = item
	}

	deleted, failed, skipped := 0, 0, 0
	fullRepository := projectSlug + "/" + target.Name
	for i := range run.Items {
		item := &run.Items[i]
		expected := previewItems[item.Digest]
		current, reconcileErr := s.inventory.Reconcile(ctx, projectID, projectSlug, target.Name)
		currentByDigest := map[string]registrydomain.ManifestInventory{}
		for _, manifest := range current {
			currentByDigest[manifest.Digest] = manifest
		}
		freshTarget, repositoryErr := s.store.FindRepositoryByID(ctx, target.ID)
		var currentDecisions []registrydomain.LifecyclePreviewItem
		var decisionErr error
		if repositoryErr == nil && reconcileErr == nil {
			currentDecisions, decisionErr = evaluateLifecycle(freshTarget.Policies, current, s.now())
		}
		eligible := map[string]registrydomain.LifecyclePreviewItem{}
		for _, decision := range currentDecisions {
			if decision.Decision == constants.LifecycleDecisionEligible {
				eligible[decision.Digest] = decision
			}
		}
		currentItem, stillEligible := eligible[item.Digest]
		if reconcileErr != nil || repositoryErr != nil || decisionErr != nil || !stillEligible ||
			!equalStrings(expected.Tags, currentItem.Tags) {
			item.Status = constants.LifecycleItemSkipped
			item.Message = "artifact changed or is no longer eligible; create a new preview"
			skipped++
			item.UpdatedAt = s.now()
			if err := s.store.UpdateLifecycleRunItem(ctx, item); err != nil {
				return nil, s.failLifecycleRun(ctx, run, preview.ID, err)
			}
			if s.audit != nil {
				if err := s.audit.Record(ctx, actor, constants.AuditLifecycleItemSkipped,
					constants.AuditResourceLifecycleRun, run.ID,
					map[string]any{"digest": item.Digest, "message": item.Message}); err != nil {
					return nil, s.failLifecycleRun(ctx, run, preview.ID, err)
				}
			}
			continue
		}
		if _, exists := currentByDigest[item.Digest]; !exists {
			item.Status = constants.LifecycleItemSkipped
			item.Message = "artifact is no longer present"
			skipped++
		} else if err := s.distribution.DeleteManifest(ctx, fullRepository, item.Digest); err != nil {
			item.Status = constants.LifecycleItemFailed
			item.Message = err.Error()
			failed++
		} else {
			item.Status = constants.LifecycleItemDeleted
			item.Message = "manifest deleted; storage is reclaimed by a later garbage collection"
			deleted++
			if err := s.store.MarkManifestDeleted(ctx, target.ID, item.Digest, s.now()); err != nil {
				item.Status = constants.LifecycleItemFailed
				item.Message = "manifest deleted but inventory update failed"
				deleted--
				failed++
			}
		}
		item.UpdatedAt = s.now()
		if err := s.store.UpdateLifecycleRunItem(ctx, item); err != nil {
			return nil, s.failLifecycleRun(ctx, run, preview.ID, err)
		}
		action := constants.AuditLifecycleItemDeleted
		if item.Status == constants.LifecycleItemFailed {
			action = constants.AuditLifecycleItemFailed
		} else if item.Status != constants.LifecycleItemDeleted {
			action = constants.AuditLifecycleItemSkipped
		}
		if s.audit != nil {
			if err := s.audit.Record(ctx, actor, action, constants.AuditResourceLifecycleRun, run.ID,
				map[string]any{"digest": item.Digest, "status": item.Status, "message": item.Message}); err != nil {
				return nil, s.failLifecycleRun(ctx, run, preview.ID, err)
			}
		}
	}

	status := constants.LifecycleRunCompleted
	if failed > 0 && deleted == 0 {
		status = constants.LifecycleRunFailed
	} else if failed > 0 || skipped > 0 {
		status = constants.LifecycleRunPartiallyCompleted
	}
	completedAt := s.now()
	run.Status = status
	run.CompletedAt = &completedAt
	if err := s.store.CompleteLifecycleRun(ctx, run.ID, status, completedAt); err != nil {
		return nil, err
	}
	if err := s.store.SetLifecyclePreviewStatus(ctx, preview.ID, constants.LifecyclePreviewExecuted); err != nil {
		return nil, err
	}
	action := constants.AuditLifecycleRunCompleted
	if status == constants.LifecycleRunFailed {
		action = constants.AuditLifecycleRunFailed
	}
	if s.audit != nil {
		if err := s.audit.Record(ctx, actor, action, constants.AuditResourceLifecycleRun, run.ID,
			map[string]any{"deleted": deleted, "failed": failed, "skipped": skipped, "status": status}); err != nil {
			return run, err
		}
	}
	return run, nil
}

func (s *LifecycleService) failLifecycleRun(
	ctx context.Context,
	run *registrydomain.LifecycleRun,
	previewID foundation.ID,
	cause error,
) error {
	completedAt := s.now()
	run.Status = constants.LifecycleRunFailed
	run.CompletedAt = &completedAt
	if err := s.store.CompleteLifecycleRun(ctx, run.ID, run.Status, completedAt); err != nil {
		return fmt.Errorf("%v; additionally failed to persist lifecycle failure: %w", cause, err)
	}
	if err := s.store.SetLifecyclePreviewStatus(ctx, previewID, constants.LifecyclePreviewExecuted); err != nil {
		return fmt.Errorf("%v; additionally failed to consume lifecycle preview: %w", cause, err)
	}
	return cause
}

func equalStrings(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *LifecycleService) FindPreview(ctx context.Context, previewID foundation.ID) (*registrydomain.LifecyclePreview, error) {
	preview, err := s.store.FindLifecyclePreview(ctx, previewID)
	if err != nil {
		return nil, err
	}
	if preview.Status == constants.LifecyclePreviewReady && !s.now().Before(preview.ExpiresAt) {
		preview.Status = constants.LifecyclePreviewExpired
		if err := s.store.SetLifecyclePreviewStatus(ctx, preview.ID, preview.Status); err != nil {
			return nil, err
		}
	}
	return preview, nil
}

func (s *LifecycleService) ListRuns(ctx context.Context, projectID foundation.ID, repository string) ([]registrydomain.LifecycleRun, error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	runs, err := s.store.ListLifecycleRuns(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	for i := range runs {
		runs[i].Repository = repository
	}
	return runs, nil
}

func (s *LifecycleService) FindRun(ctx context.Context, runID foundation.ID) (*registrydomain.LifecycleRun, error) {
	return s.store.FindLifecycleRun(ctx, runID)
}
