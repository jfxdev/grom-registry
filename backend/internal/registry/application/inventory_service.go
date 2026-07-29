package application

import (
	"context"
	"fmt"
	"strings"
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
}

func NewInventoryService(store registrydomain.Store) *InventoryService {
	return &InventoryService{store: store, now: func() time.Time { return time.Now().UTC() }}
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
		return err
	}
	now := s.now()
	classification := ClassifyManifest(*metadata)
	observation := registrydomain.ManifestObservation{
		Digest: metadata.Digest, MediaType: metadata.MediaType, ArtifactType: metadata.ArtifactType,
		SubjectDigest: metadata.SubjectDigest, ManifestSize: metadata.ManifestSize,
		ObservedKind: classification.Kind, ArtifactRelationship: classification.Relationship,
		ClassificationSource: classification.Source, ClassificationConfidence: classification.Confidence,
	}
	if !strings.Contains(reference, ":") {
		observation.Tag = reference
		observation.PushedAt = &now
	}
	if err := s.store.UpsertManifestObservation(ctx, target.ID, observation, now); err != nil {
		return err
	}
	if observation.Tag != "" && classification.Relationship == constants.ArtifactRelationshipPrimary &&
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
		return nil, err
	}
	now := s.now()
	seenTags := make([]string, 0, len(tags))
	seenDigests := make([]string, 0, len(tags))
	digests := map[string]struct{}{}
	for _, tag := range tags {
		metadata, fetchErr := s.distribution.FetchManifest(ctx, fullRepository, tag)
		if fetchErr != nil {
			return nil, fetchErr
		}
		observation := registrydomain.ManifestObservation{
			Digest: metadata.Digest, MediaType: metadata.MediaType, ArtifactType: metadata.ArtifactType,
			SubjectDigest: metadata.SubjectDigest, ManifestSize: metadata.ManifestSize, Tag: tag,
		}
		classification := ClassifyManifest(*metadata)
		observation.ObservedKind = classification.Kind
		observation.ArtifactRelationship = classification.Relationship
		observation.ClassificationSource = classification.Source
		observation.ClassificationConfidence = classification.Confidence
		if err := s.store.UpsertManifestObservation(ctx, target.ID, observation, now); err != nil {
			return nil, err
		}
		if classification.Relationship == constants.ArtifactRelationshipPrimary &&
			registrydomain.ApplyInferredProfile(target, classification.Profile, classification.Confidence, now) {
			if err := s.store.SaveRepositoryProfile(ctx, target); err != nil {
				return nil, err
			}
		}
		seenTags = append(seenTags, tag)
		if _, exists := digests[metadata.Digest]; !exists {
			digests[metadata.Digest] = struct{}{}
			seenDigests = append(seenDigests, metadata.Digest)
		}
	}
	for _, subjectDigest := range append([]string(nil), seenDigests...) {
		referrers, referrerErr := s.distribution.ListReferrers(ctx, fullRepository, subjectDigest)
		if referrerErr != nil {
			return nil, referrerErr
		}
		for _, descriptor := range referrers {
			metadata, fetchErr := s.distribution.FetchManifest(ctx, fullRepository, descriptor.Digest)
			if fetchErr != nil {
				return nil, fetchErr
			}
			observation := registrydomain.ManifestObservation{
				Digest: metadata.Digest, MediaType: metadata.MediaType, ArtifactType: metadata.ArtifactType,
				SubjectDigest: metadata.SubjectDigest, ManifestSize: metadata.ManifestSize,
			}
			classification := ClassifyManifest(*metadata)
			observation.ObservedKind = classification.Kind
			observation.ArtifactRelationship = classification.Relationship
			observation.ClassificationSource = classification.Source
			observation.ClassificationConfidence = classification.Confidence
			if err := s.store.UpsertManifestObservation(ctx, target.ID, observation, now); err != nil {
				return nil, err
			}
			if _, exists := digests[metadata.Digest]; !exists {
				digests[metadata.Digest] = struct{}{}
				seenDigests = append(seenDigests, metadata.Digest)
			}
		}
	}
	if err := s.store.CompleteInventoryReconciliation(ctx, target.ID, seenTags, seenDigests, now); err != nil {
		return nil, err
	}
	return s.store.ListManifestInventory(ctx, target.ID)
}

func (s *InventoryService) List(ctx context.Context, projectID foundation.ID, repository string) ([]registrydomain.ManifestInventory, error) {
	target, err := s.store.FindRepository(ctx, projectID, repository)
	if err != nil {
		return nil, err
	}
	return s.store.ListManifestInventory(ctx, target.ID)
}
