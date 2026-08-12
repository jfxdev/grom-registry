package bunstore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

const storageReady = "ready"
const storageStale = "stale"

// StorageUsageForProject is intentionally a Registry-owned query. Projects
// calls it through InventoryService rather than reading Registry tables.
func (s *Store) StorageUsageForProject(ctx context.Context, projectID foundation.ID) (foundation.AccountedStorageUsage, error) {
	model := new(projectStorageSnapshotModel)
	err := s.db.NewSelect().Model(model).Where("project_id = ?", projectID.String()).Scan(ctx)
	if err == nil {
		return storageUsage(model.Status, model.AccountedBytes, model.ReconciledAt), nil
	}
	if err == sql.ErrNoRows {
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
	if err == sql.ErrNoRows {
		return foundation.PendingStorageUsage(), nil
	}
	return foundation.AccountedStorageUsage{Status: "unavailable"}, err
}

func storageUsage(status string, bytes int64, reconciledAt time.Time) foundation.AccountedStorageUsage {
	return foundation.AccountedStorageUsage{Status: status, AccountedBytes: &bytes, ReconciledAt: &reconciledAt}
}

func (s *Store) attachStorageUsage(ctx context.Context, repository *registrydomain.Repository) error {
	usage, err := s.storageUsageForRepository(ctx, repository.ID)
	if err != nil {
		return err
	}
	repository.AccountedUsage = usage
	return nil
}

func (s *Store) upsertManifestStorageFacts(ctx context.Context, tx bun.Tx, repositoryID foundation.ID, observation registrydomain.ManifestObservation, observedAt time.Time) error {
	descriptors, err := normalizedDescriptors(observation)
	if err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		existing := new(blobDescriptorModel)
		err := tx.NewSelect().Model(existing).Where("digest = ?", descriptor.Digest).Scan(ctx)
		switch {
		case err == nil:
			if existing.SizeBytes != descriptor.SizeBytes {
				return fmt.Errorf("descriptor %s was observed with inconsistent sizes", descriptor.Digest)
			}
			if _, err = tx.NewUpdate().Model((*blobDescriptorModel)(nil)).
				Set("last_seen_at = ?", observedAt).
				Where("digest = ?", descriptor.Digest).Exec(ctx); err != nil {
				return err
			}
		case err == sql.ErrNoRows:
			if _, err = tx.NewInsert().Model(&blobDescriptorModel{Digest: descriptor.Digest, SizeBytes: descriptor.SizeBytes, MediaType: descriptor.MediaType, FirstSeenAt: observedAt, LastSeenAt: observedAt}).Exec(ctx); err != nil {
				return err
			}
		default:
			return err
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
	return nil
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
	bytes, err := accountedBytes(ctx, tx, "rr.id = ?", repositoryID.String())
	if err != nil {
		return err
	}
	version := at.UnixNano()
	if _, err := tx.NewInsert().Model(&repositoryStorageSnapshotModel{RepositoryID: repositoryID.String(), AccountedBytes: bytes, InventoryVersion: version, ReconciledAt: at, Status: storageReady}).
		On("CONFLICT (repository_id) DO UPDATE").Set("accounted_bytes = EXCLUDED.accounted_bytes").Set("inventory_version = EXCLUDED.inventory_version").Set("reconciled_at = EXCLUDED.reconciled_at").Set("status = EXCLUDED.status").Exec(ctx); err != nil {
		return err
	}
	return s.refreshProjectStorageSnapshot(ctx, tx, foundation.ID(projectID), at)
}

func (s *Store) refreshProjectStorageSnapshot(ctx context.Context, tx bun.Tx, projectID foundation.ID, at time.Time) error {
	projectBytes, err := accountedBytes(ctx, tx, "rr.project_id = ?", projectID.String())
	if err != nil {
		return err
	}
	version := at.UnixNano()
	_, err = tx.NewInsert().Model(&projectStorageSnapshotModel{ProjectID: projectID.String(), AccountedBytes: projectBytes, AccountingVersion: version, ReconciledAt: at, Status: storageReady}).
		On("CONFLICT (project_id) DO UPDATE").Set("accounted_bytes = EXCLUDED.accounted_bytes").Set("accounting_version = EXCLUDED.accounting_version").Set("reconciled_at = EXCLUDED.reconciled_at").Set("status = EXCLUDED.status").Exec(ctx)
	return err
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
		for _, project := range projects {
			if err := s.refreshProjectStorageSnapshot(ctx, tx, foundation.ID(project.ID), at); err != nil {
				return err
			}
		}
		return nil
	})
}

func accountedBytes(ctx context.Context, tx bun.Tx, scope string, value any) (int64, error) {
	var rows []blobDescriptorModel
	err := tx.NewSelect().Model(&rows).Distinct().
		Join("JOIN registry_manifest_blob_references AS rmbr ON rmbr.blob_digest = rbd.digest").
		Join("JOIN registry_repositories AS rr ON rr.id = rmbr.repository_id").
		Join("JOIN registry_manifests AS rm ON rm.repository_id = rmbr.repository_id AND rm.digest = rmbr.manifest_digest").
		Where(scope, value).
		Where("rm.state IN (?, ?)", constants.InventoryStateActive, constants.InventoryStateUntagged).
		Scan(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, row := range rows {
		if row.SizeBytes > math.MaxInt64-total {
			return 0, fmt.Errorf("accounted storage exceeds signed 64-bit range")
		}
		total += row.SizeBytes
	}
	return total, nil
}

func (s *Store) MarkRepositoryStorageStale(ctx context.Context, repositoryID foundation.ID) error {
	_, err := s.db.NewUpdate().Model((*repositoryStorageSnapshotModel)(nil)).Set("status = ?", storageStale).Where("repository_id = ?", repositoryID.String()).Exec(ctx)
	if err != nil {
		return err
	}
	var projectID string
	if err := s.db.NewSelect().Model((*repositoryModel)(nil)).Column("project_id").Where("id = ?", repositoryID.String()).Scan(ctx, &projectID); err != nil {
		return err
	}
	_, err = s.db.NewUpdate().Model((*projectStorageSnapshotModel)(nil)).Set("status = ?", storageStale).Where("project_id = ?", projectID).Exec(ctx)
	return err
}
