package bunstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

const storageReady = "ready"
const storageStale = "stale"
const storagePending = "pending"

// StorageUsageForProject is intentionally a Registry-owned query. Projects
// calls it through InventoryService rather than reading Registry tables.
func (s *Store) StorageUsageForProject(ctx context.Context, projectID foundation.ID) (foundation.AccountedStorageUsage, error) {
	model := new(projectStorageSnapshotModel)
	err := s.db.NewSelect().Model(model).Where("project_id = ?", projectID.String()).Scan(ctx)
	if err == nil {
		return storageUsage(model.Status, model.AccountedBytes, model.ReconciledAt), nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return foundation.PendingStorageUsage(), nil
	}
	return foundation.AccountedStorageUsage{Status: "unavailable"}, err
}

func (s *Store) storageUsageForRepository(ctx context.Context, repositoryID foundation.ID) (foundation.AccountedStorageUsage, error) {
	model := new(repositoryStorageSnapshotModel)
	err := s.db.NewSelect().Model(model).Where("repository_id = ?", repositoryID.String()).Scan(ctx)
	if err == nil {
		return storageUsage(model.Status, model.AccountedBytes, model.ReconciledAt), nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return foundation.PendingStorageUsage(), nil
	}
	return foundation.AccountedStorageUsage{Status: "unavailable"}, err
}

func storageUsage(status string, bytes int64, reconciledAt time.Time) foundation.AccountedStorageUsage {
	return foundation.AccountedStorageUsage{Status: status, AccountedBytes: &bytes, ReconciledAt: &reconciledAt}
}

func (s *Store) attachStorageUsage(ctx context.Context, repository *registrydomain.Repository) {
	usage, err := s.storageUsageForRepository(ctx, repository.ID)
	if err != nil {
		repository.AccountedUsage = foundation.AccountedStorageUsage{Status: "unavailable"}
		return
	}
	repository.AccountedUsage = usage
}

func (s *Store) attachStorageUsages(ctx context.Context, repositories []*registrydomain.Repository) {
	if len(repositories) == 0 {
		return
	}
	ids := make([]string, len(repositories))
	for i, repository := range repositories {
		ids[i] = repository.ID.String()
	}
	var snapshots []repositoryStorageSnapshotModel
	if err := s.db.NewSelect().Model(&snapshots).Where("repository_id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		for _, repository := range repositories {
			repository.AccountedUsage = foundation.AccountedStorageUsage{Status: "unavailable"}
		}
		return
	}
	byID := make(map[string]repositoryStorageSnapshotModel, len(snapshots))
	for _, snapshot := range snapshots {
		byID[snapshot.RepositoryID] = snapshot
	}
	for _, repository := range repositories {
		snapshot, ok := byID[repository.ID.String()]
		if !ok {
			repository.AccountedUsage = foundation.PendingStorageUsage()
			continue
		}
		repository.AccountedUsage = storageUsage(snapshot.Status, snapshot.AccountedBytes, snapshot.ReconciledAt)
	}
}

func (s *Store) StorageUsageForProjects(ctx context.Context, projectIDs []foundation.ID) (map[foundation.ID]foundation.AccountedStorageUsage, error) {
	result := make(map[foundation.ID]foundation.AccountedStorageUsage, len(projectIDs))
	if len(projectIDs) == 0 {
		return result, nil
	}
	ids := make([]string, len(projectIDs))
	for i, projectID := range projectIDs {
		ids[i] = projectID.String()
		result[projectID] = foundation.PendingStorageUsage()
	}
	var snapshots []projectStorageSnapshotModel
	if err := s.db.NewSelect().Model(&snapshots).Where("project_id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		result[foundation.ID(snapshot.ProjectID)] = storageUsage(snapshot.Status, snapshot.AccountedBytes, snapshot.ReconciledAt)
	}
	return result, nil
}

func (s *Store) upsertManifestStorageFacts(ctx context.Context, tx bun.Tx, repositoryID foundation.ID, observation registrydomain.ManifestObservation, observedAt time.Time) error {
	descriptors, err := normalizedDescriptors(observation)
	if err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		var canonicalSize int64
		if err := tx.NewInsert().Model(&blobDescriptorModel{Digest: descriptor.Digest, SizeBytes: descriptor.SizeBytes, MediaType: descriptor.MediaType, FirstSeenAt: observedAt, LastSeenAt: observedAt}).
			On("CONFLICT (digest) DO UPDATE").Set("last_seen_at = EXCLUDED.last_seen_at").Returning("size_bytes").Scan(ctx, &canonicalSize); err != nil {
			return err
		}
		if canonicalSize != descriptor.SizeBytes {
			return fmt.Errorf("descriptor %s was observed with inconsistent sizes", descriptor.Digest)
		}
	}
	if _, err := tx.NewDelete().Model((*manifestBlobReferenceModel)(nil)).
		Where("repository_id = ?", repositoryID.String()).Where("manifest_digest = ?", observation.Digest).Exec(ctx); err != nil {
		return err
	}
	references := make([]manifestBlobReferenceModel, 0, len(descriptors))
	for _, descriptor := range descriptors {
		references = append(references, manifestBlobReferenceModel{RepositoryID: repositoryID.String(), ManifestDigest: observation.Digest, BlobDigest: descriptor.Digest, Role: descriptor.Role})
	}
	if len(references) > 0 {
		if _, err := tx.NewInsert().Model(&references).Exec(ctx); err != nil {
			return err
		}
	}
	return deleteUnreferencedBlobDescriptors(ctx, tx)
}

func normalizedDescriptors(observation registrydomain.ManifestObservation) ([]registrydomain.Descriptor, error) {
	descriptors := append([]registrydomain.Descriptor(nil), observation.Descriptors...)
	descriptors = append(descriptors, registrydomain.Descriptor{Digest: observation.Digest, SizeBytes: observation.ManifestSize, MediaType: observation.MediaType, Role: "manifest"})
	byDigest := make(map[string]registrydomain.Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		if strings.TrimSpace(descriptor.Digest) == "" || descriptor.SizeBytes < 0 {
			return nil, fmt.Errorf("invalid observed OCI descriptor")
		}
		if descriptor.Role == "" {
			descriptor.Role = "descriptor"
		}
		if existing, found := byDigest[descriptor.Digest]; found {
			if existing.SizeBytes != descriptor.SizeBytes {
				return nil, fmt.Errorf("descriptor %s has inconsistent sizes in one manifest", descriptor.Digest)
			}
			continue
		}
		byDigest[descriptor.Digest] = descriptor
	}
	result := make([]registrydomain.Descriptor, 0, len(byDigest))
	for _, descriptor := range byDigest {
		result = append(result, descriptor)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Digest < result[j].Digest })
	return result, nil
}

func (s *Store) refreshStorageSnapshots(ctx context.Context, tx bun.Tx, repositoryID foundation.ID, at time.Time) error {
	var projectID string
	if err := tx.NewSelect().Model((*repositoryModel)(nil)).Column("project_id").Where("id = ?", repositoryID.String()).Scan(ctx, &projectID); err != nil {
		return err
	}
	if err := s.refreshRepositoryStorageSnapshot(ctx, tx, repositoryID, at); err != nil {
		return err
	}
	return s.refreshProjectStorageSnapshot(ctx, tx, foundation.ID(projectID), at)
}

func (s *Store) refreshRepositoryStorageSnapshot(ctx context.Context, tx bun.Tx, repositoryID foundation.ID, at time.Time) error {
	bytes, err := accountedBytes(ctx, tx, "rr.id = ?", repositoryID.String())
	if err != nil {
		return err
	}
	version, err := nextRepositoryStorageVersion(ctx, tx, repositoryID, at.UnixNano())
	if err != nil {
		return err
	}
	if _, err := tx.NewInsert().Model(&repositoryStorageSnapshotModel{RepositoryID: repositoryID.String(), AccountedBytes: bytes, InventoryVersion: version, ReconciledAt: at, Status: storageReady}).
		On("CONFLICT (repository_id) DO UPDATE").
		Set("accounted_bytes = CASE WHEN EXCLUDED.inventory_version > inventory_version THEN EXCLUDED.accounted_bytes ELSE accounted_bytes END").
		Set("inventory_version = CASE WHEN EXCLUDED.inventory_version > inventory_version THEN EXCLUDED.inventory_version ELSE inventory_version END").
		Set("reconciled_at = CASE WHEN EXCLUDED.inventory_version > inventory_version THEN EXCLUDED.reconciled_at ELSE reconciled_at END").
		Set("status = CASE WHEN EXCLUDED.inventory_version > inventory_version THEN EXCLUDED.status ELSE status END").Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) refreshProjectStorageSnapshot(ctx context.Context, tx bun.Tx, projectID foundation.ID, at time.Time) error {
	if err := s.lockProjectStorageSnapshot(ctx, tx, projectID); err != nil {
		return err
	}
	projectBytes, err := accountedBytes(ctx, tx, "rr.project_id = ?", projectID.String())
	if err != nil {
		return err
	}
	version, err := nextProjectStorageVersion(ctx, tx, projectID, at.UnixNano())
	if err != nil {
		return err
	}
	status, err := projectStorageStatus(ctx, tx, projectID)
	if err != nil {
		return err
	}
	_, err = tx.NewInsert().Model(&projectStorageSnapshotModel{ProjectID: projectID.String(), AccountedBytes: projectBytes, AccountingVersion: version, ReconciledAt: at, Status: status}).
		On("CONFLICT (project_id) DO UPDATE").
		Set("accounted_bytes = CASE WHEN EXCLUDED.accounting_version > accounting_version THEN EXCLUDED.accounted_bytes ELSE accounted_bytes END").
		Set("accounting_version = CASE WHEN EXCLUDED.accounting_version > accounting_version THEN EXCLUDED.accounting_version ELSE accounting_version END").
		Set("reconciled_at = CASE WHEN EXCLUDED.accounting_version > accounting_version THEN EXCLUDED.reconciled_at ELSE reconciled_at END").
		Set("status = CASE WHEN EXCLUDED.accounting_version > accounting_version THEN EXCLUDED.status ELSE status END").Exec(ctx)
	return err
}

func (s *Store) lockProjectStorageSnapshot(ctx context.Context, tx bun.Tx, projectID foundation.ID) error {
	if s.db.Dialect().Name() == dialect.PG {
		var id string
		return tx.NewSelect().Table("projects").Column("id").Where("id = ?", projectID.String()).For("UPDATE").Scan(ctx, &id)
	}
	result, err := tx.NewUpdate().Table("projects").Set("slug = slug").Where("id = ?", projectID.String()).Exec(ctx)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func nextRepositoryStorageVersion(ctx context.Context, tx bun.Tx, repositoryID foundation.ID, requested int64) (int64, error) {
	var current int64
	err := tx.NewSelect().Model((*repositoryStorageSnapshotModel)(nil)).Column("inventory_version").Where("repository_id = ?", repositoryID.String()).Scan(ctx, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return requested, nil
	}
	if err != nil {
		return 0, err
	}
	return nextStorageVersion(requested, current)
}

func nextProjectStorageVersion(ctx context.Context, tx bun.Tx, projectID foundation.ID, requested int64) (int64, error) {
	var current int64
	err := tx.NewSelect().Model((*projectStorageSnapshotModel)(nil)).Column("accounting_version").Where("project_id = ?", projectID.String()).Scan(ctx, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return requested, nil
	}
	if err != nil {
		return 0, err
	}
	return nextStorageVersion(requested, current)
}

func nextStorageVersion(requested, current int64) (int64, error) {
	if requested != current {
		return requested, nil
	}
	if current == math.MaxInt64 {
		return 0, fmt.Errorf("storage accounting version exceeds signed 64-bit range")
	}
	return current + 1, nil
}

func projectStorageStatus(ctx context.Context, tx bun.Tx, projectID foundation.ID) (string, error) {
	var repositories []repositoryModel
	if err := tx.NewSelect().Model(&repositories).Where("project_id = ?", projectID.String()).Scan(ctx); err != nil {
		return "", err
	}
	if len(repositories) == 0 {
		return storageReady, nil
	}
	ids := make([]string, len(repositories))
	for i, repository := range repositories {
		ids[i] = repository.ID
	}
	var inventoryRepositoryIDs []string
	if err := tx.NewSelect().Table("registry_manifests").ColumnExpr("DISTINCT repository_id").Where("repository_id IN (?)", bun.List(ids)).Scan(ctx, &inventoryRepositoryIDs); err != nil {
		return "", err
	}
	var snapshots []repositoryStorageSnapshotModel
	if err := tx.NewSelect().Model(&snapshots).Where("repository_id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return "", err
	}
	byRepositoryID := make(map[string]repositoryStorageSnapshotModel, len(snapshots))
	for _, snapshot := range snapshots {
		byRepositoryID[snapshot.RepositoryID] = snapshot
		if snapshot.Status == storageStale {
			return storageStale, nil
		}
	}
	for _, repositoryID := range inventoryRepositoryIDs {
		_, found := byRepositoryID[repositoryID]
		if !found {
			return storagePending, nil
		}
	}
	return storageReady, nil
}

// RebuildRepositoryStorage is an idempotent repair primitive. It never reads
// Distribution storage; the aggregate is always rebuilt from Registry facts.
func (s *Store) RebuildRepositoryStorage(ctx context.Context, repositoryID foundation.ID, at time.Time) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return s.refreshStorageSnapshots(ctx, tx, repositoryID, at)
	})
}

func (s *Store) RebuildProjectStorage(ctx context.Context, projectID foundation.ID, at time.Time) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return s.refreshProjectStorageSnapshot(ctx, tx, projectID, at)
	})
}

func (s *Store) RebuildAllStorage(ctx context.Context, at time.Time) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var projects []struct {
			ID string `bun:"id"`
		}
		if err := tx.NewSelect().Table("projects").Column("id").OrderExpr("id ASC").Scan(ctx, &projects); err != nil {
			return err
		}
		for projectIndex, project := range projects {
			var repositories []repositoryModel
			if err := tx.NewSelect().Model(&repositories).Where("project_id = ?", project.ID).OrderExpr("id ASC").Scan(ctx); err != nil {
				return err
			}
			for repositoryIndex, repository := range repositories {
				reconciledAt := at.Add(time.Duration(projectIndex+repositoryIndex+1) * time.Nanosecond)
				if err := s.refreshRepositoryStorageSnapshot(ctx, tx, foundation.ID(repository.ID), reconciledAt); err != nil {
					return err
				}
			}
			projectAt := at.Add(time.Duration(projectIndex+len(repositories)+2) * time.Nanosecond)
			if err := s.refreshProjectStorageSnapshot(ctx, tx, foundation.ID(project.ID), projectAt); err != nil {
				return err
			}
		}
		return nil
	})
}

func accountedBytes(ctx context.Context, tx bun.Tx, scope string, value any) (int64, error) {
	query := `SELECT COALESCE(SUM(accounted_descriptors.size_bytes), 0)
		FROM (SELECT DISTINCT rbd.digest, rbd.size_bytes
			FROM registry_blob_descriptors AS rbd
			JOIN registry_manifest_blob_references AS rmbr ON rmbr.blob_digest = rbd.digest
			JOIN registry_repositories AS rr ON rr.id = rmbr.repository_id
			JOIN registry_manifests AS rm ON rm.repository_id = rmbr.repository_id AND rm.digest = rmbr.manifest_digest
			WHERE ` + scope + ` AND rm.state IN (?, ?)) AS accounted_descriptors`
	var total int64
	if err := tx.NewRaw(query, value, constants.InventoryStateActive, constants.InventoryStateUntagged).Scan(ctx, &total); err != nil {
		return 0, err
	}
	if total < 0 {
		return 0, fmt.Errorf("accounted storage exceeds signed 64-bit range")
	}
	return total, nil
}

func (s *Store) MarkRepositoryStorageStale(ctx context.Context, repositoryID foundation.ID) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var projectID string
		if err := tx.NewSelect().Model((*repositoryModel)(nil)).Column("project_id").Where("id = ?", repositoryID.String()).Scan(ctx, &projectID); err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model((*repositoryStorageSnapshotModel)(nil)).Set("status = ?", storageStale).Where("repository_id = ?", repositoryID.String()).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*projectStorageSnapshotModel)(nil)).Set("status = ?", storageStale).Where("project_id = ?", projectID).Exec(ctx)
		return err
	})
}
