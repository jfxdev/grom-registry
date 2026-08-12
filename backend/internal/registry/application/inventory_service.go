package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type ManifestMetadata struct {
	Digest               string
	MediaType            string
	ArtifactType         string
	SubjectDigest        string
	ManifestSize         int64
	ConfigMediaType      string
	LayerMediaTypes      []string
	DescriptorMediaTypes []string
	Platforms            []registrydomain.ManifestPlatform
	Children             []ManifestMetadata
	Descriptors          []registrydomain.Descriptor
}

type storageStaleMarker interface {
	MarkRepositoryStorageStale(context.Context, foundation.ID) error
}

type ManifestDescriptor struct {
	Digest       string
	MediaType    string
	ArtifactType string
	Size         int64
}

type DistributionMetadata interface {
	ListRepositoryTags(ctx context.Context, repository string) ([]string, error)
	ResolveManifest(ctx context.Context, repository, reference string) (string, bool, error)
	FetchManifest(ctx context.Context, repository, reference string) (*ManifestMetadata, error)
	ListReferrers(ctx context.Context, repository, digest string) ([]ManifestDescriptor, error)
	DeleteManifest(ctx context.Context, repository, digest string) error
}

type InventoryService struct {
	store          registrydomain.Store
	distribution   DistributionMetadata
	resolveProject func(context.Context, string) (foundation.ID, error)
	now            func() time.Time
	missingProbeMu sync.Mutex
	missingProbes  map[string]time.Time
}

const missingManifestProbeInterval = time.Hour

func NewInventoryService(store registrydomain.Store) *InventoryService {
	return &InventoryService{store: store, now: func() time.Time { return time.Now().UTC() }, missingProbes: make(map[string]time.Time)}
}

func (s *InventoryService) SetDistribution(distribution DistributionMetadata) {
	s.distribution = distribution
}

func (s *InventoryService) SetProjectResolver(resolver func(context.Context, string) (foundation.ID, error)) {
	s.resolveProject = resolver
}

func (s *InventoryService) ObservePush(ctx context.Context, fullRepository, reference, digest string) error {
	if s.distribution == nil || s.resolveProject == nil {
		return fmt.Errorf("inventory service is not configured")
	}
	projectSlug, repository, ok := strings.Cut(fullRepository, "/")
	if !ok || projectSlug == "" || repository == "" {
		return fmt.Errorf("invalid project repository")
	}
	projectID, err := s.resolveProject(ctx, projectSlug)
	if err != nil {
		return err
	}
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return err
	}
	manifestReference := digest
	if manifestReference == "" {
		manifestReference = reference
	}
	metadata, err := s.distribution.FetchManifest(ctx, fullRepository, manifestReference)
	if err != nil {
		s.markStorageStale(ctx, target.ID)
		return err
	}
	now := s.now()
	classification := ClassifyManifest(*metadata)
	tag := ""
	var pushedAt *time.Time
	if !strings.Contains(reference, ":") {
		tag = reference
		pushedAt = &now
	}
	if _, err := s.upsertMetadataTree(ctx, target.ID, metadata, tag, pushedAt, now); err != nil {
		s.markStorageStale(ctx, target.ID)
		return err
	}
	if tag != "" && classification.Relationship == constants.ArtifactRelationshipPrimary &&
		registrydomain.ApplyInferredProfile(target, classification.Profile, classification.Confidence, now) {
		return s.store.SaveRepositoryProfile(ctx, target)
	}
	return nil
}

func (s *InventoryService) Reconcile(
	ctx context.Context,
	projectID foundation.ID,
	projectSlug, repository string,
) ([]registrydomain.ManifestInventory, error) {
	if s.distribution == nil {
		return nil, fmt.Errorf("inventory service is not configured")
	}
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	fullRepository := projectSlug + "/" + repository
	tags, err := s.distribution.ListRepositoryTags(ctx, fullRepository)
	if err != nil {
		s.markStorageStale(ctx, target.ID)
		return nil, err
	}
	sort.Strings(tags)
	now := s.now()
	seenTags := make([]string, 0, len(tags))
	seenDigests := make([]string, 0, len(tags))
	digests := map[string]struct{}{}
	for _, tag := range tags {
		metadata, fetchErr := s.distribution.FetchManifest(ctx, fullRepository, tag)
		if fetchErr != nil {
			s.markStorageStale(ctx, target.ID)
			return nil, fetchErr
		}
		classification := ClassifyManifest(*metadata)
		observedDigests, err := s.upsertMetadataTree(ctx, target.ID, metadata, tag, nil, now)
		if err != nil {
			s.markStorageStale(ctx, target.ID)
			return nil, err
		}
		if classification.Relationship == constants.ArtifactRelationshipPrimary &&
			registrydomain.ApplyInferredProfile(target, classification.Profile, classification.Confidence, now) {
			if err := s.store.SaveRepositoryProfile(ctx, target); err != nil {
				return nil, err
			}
		}
		seenTags = append(seenTags, tag)
		for _, observedDigest := range observedDigests {
			s.clearMissingProbe(target.ID.String() + ":" + observedDigest)
			if _, exists := digests[observedDigest]; !exists {
				digests[observedDigest] = struct{}{}
				seenDigests = append(seenDigests, observedDigest)
			}
		}
	}
	known, err := s.store.ListManifestInventory(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	for _, manifest := range known {
		if manifest.State == constants.InventoryStateDeleted {
			continue
		}
		if _, exists := digests[manifest.Digest]; exists {
			continue
		}
		probeKey := target.ID.String() + ":" + manifest.Digest
		if manifest.State == constants.InventoryStateMissing && !s.shouldProbeMissing(probeKey, now) {
			continue
		}
		resolved, exists, resolveErr := s.distribution.ResolveManifest(ctx, fullRepository, manifest.Digest)
		if resolveErr != nil {
			s.markStorageStale(ctx, target.ID)
			s.clearMissingProbe(probeKey)
			return nil, resolveErr
		}
		if !exists || resolved != manifest.Digest {
			continue
		}
		s.clearMissingProbe(probeKey)
		metadata, fetchErr := s.distribution.FetchManifest(ctx, fullRepository, manifest.Digest)
		if fetchErr != nil {
			s.markStorageStale(ctx, target.ID)
			return nil, fetchErr
		}
		observedDigests, err := s.upsertMetadataTree(ctx, target.ID, metadata, "", nil, now)
		if err != nil {
			s.markStorageStale(ctx, target.ID)
			return nil, err
		}
		for _, observedDigest := range observedDigests {
			if _, exists := digests[observedDigest]; !exists {
				digests[observedDigest] = struct{}{}
				seenDigests = append(seenDigests, observedDigest)
			}
		}
	}
	for i := 0; i < len(seenDigests); i++ {
		subjectDigest := seenDigests[i]
		referrers, referrerErr := s.distribution.ListReferrers(ctx, fullRepository, subjectDigest)
		if referrerErr != nil {
			s.markStorageStale(ctx, target.ID)
			return nil, referrerErr
		}
		sort.Slice(referrers, func(i, j int) bool { return referrers[i].Digest < referrers[j].Digest })
		for _, descriptor := range referrers {
			if descriptor.Digest == "" || descriptor.Size < 0 {
				s.markStorageStale(ctx, target.ID)
				return nil, fmt.Errorf("invalid referrer descriptor")
			}
			metadata, fetchErr := s.distribution.FetchManifest(ctx, fullRepository, descriptor.Digest)
			if fetchErr != nil {
				s.markStorageStale(ctx, target.ID)
				return nil, fetchErr
			}
			if metadata.Digest != descriptor.Digest || metadata.ManifestSize != descriptor.Size {
				s.markStorageStale(ctx, target.ID)
				return nil, fmt.Errorf("referrer descriptor %s has inconsistent metadata", descriptor.Digest)
			}
			observedDigests, err := s.upsertMetadataTree(ctx, target.ID, metadata, "", nil, now)
			if err != nil {
				s.markStorageStale(ctx, target.ID)
				return nil, err
			}
			for _, observedDigest := range observedDigests {
				if _, exists := digests[observedDigest]; !exists {
					digests[observedDigest] = struct{}{}
					seenDigests = append(seenDigests, observedDigest)
				}
			}
		}
	}
	if err := s.store.CompleteInventoryReconciliation(ctx, target.ID, seenTags, seenDigests, now); err != nil {
		s.markStorageStale(ctx, target.ID)
		return nil, err
	}
	return s.store.ListManifestInventory(ctx, target.ID)
}

func (s *InventoryService) shouldProbeMissing(key string, now time.Time) bool {
	s.missingProbeMu.Lock()
	defer s.missingProbeMu.Unlock()
	lastProbe, exists := s.missingProbes[key]
	if exists && now.Sub(lastProbe) < missingManifestProbeInterval {
		return false
	}
	s.missingProbes[key] = now
	return true
}

func (s *InventoryService) clearMissingProbe(key string) {
	s.missingProbeMu.Lock()
	delete(s.missingProbes, key)
	s.missingProbeMu.Unlock()
}

func (s *InventoryService) upsertMetadataTree(
	ctx context.Context,
	repositoryID foundation.ID,
	metadata *ManifestMetadata,
	tag string,
	pushedAt *time.Time,
	observedAt time.Time,
) ([]string, error) {
	digests := make([]string, 0, 1+len(metadata.Children))
	observation := observationFromMetadata(metadata, tag, pushedAt)
	if err := s.store.UpsertManifestObservation(ctx, repositoryID, observation, observedAt); err != nil {
		return nil, err
	}
	digests = append(digests, metadata.Digest)
	for i := range metadata.Children {
		childDigests, err := s.upsertMetadataTree(ctx, repositoryID, &metadata.Children[i], "", nil, observedAt)
		if err != nil {
			return nil, err
		}
		digests = append(digests, childDigests...)
	}
	return digests, nil
}

func observationFromMetadata(metadata *ManifestMetadata, tag string, pushedAt *time.Time) registrydomain.ManifestObservation {
	classification := ClassifyManifest(*metadata)
	return registrydomain.ManifestObservation{
		Digest: metadata.Digest, MediaType: metadata.MediaType, ArtifactType: metadata.ArtifactType,
		SubjectDigest: metadata.SubjectDigest, ManifestSize: metadata.ManifestSize,
		Platforms: metadata.Platforms, Tag: tag, PushedAt: pushedAt,
		Descriptors:  metadata.Descriptors,
		ObservedKind: classification.Kind, ArtifactRelationship: classification.Relationship,
		ClassificationSource: classification.Source, ClassificationConfidence: classification.Confidence,
	}
}

func (s *InventoryService) markStorageStale(ctx context.Context, repositoryID foundation.ID) {
	if marker, ok := s.store.(storageStaleMarker); ok {
		_ = marker.MarkRepositoryStorageStale(ctx, repositoryID)
	}
}

// ProjectUsage keeps the Projects context behind a narrow Registry query.
func (s *InventoryService) ProjectUsage(ctx context.Context, projectID foundation.ID) foundation.AccountedStorageUsage {
	reader, ok := s.store.(interface {
		StorageUsageForProject(context.Context, foundation.ID) (foundation.AccountedStorageUsage, error)
	})
	if !ok {
		return foundation.AccountedStorageUsage{Status: "unavailable"}
	}
	usage, err := reader.StorageUsageForProject(ctx, projectID)
	if err != nil {
		return foundation.AccountedStorageUsage{Status: "unavailable"}
	}
	return usage
}

func (s *InventoryService) List(ctx context.Context, projectID foundation.ID, repository string) ([]registrydomain.ManifestInventory, error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	return s.store.ListManifestInventory(ctx, target.ID)
}

func (s *InventoryService) ListPage(ctx context.Context, projectID foundation.ID, repository string, request foundation.PageRequest) (foundation.PageResult[registrydomain.ManifestInventory], error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return foundation.PageResult[registrydomain.ManifestInventory]{}, err
	}
	return s.store.ListManifestInventoryPage(ctx, target.ID, request)
}
