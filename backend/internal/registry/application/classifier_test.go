package application

import (
	"testing"

	"github.com/jfxdev/grom/backend/internal/constants"
)

func TestClassifyManifest(t *testing.T) {
	tests := []struct {
		name         string
		metadata     ManifestMetadata
		kind         string
		profile      string
		relationship string
		confidence   string
	}{
		{
			name:     "container image config",
			metadata: ManifestMetadata{ConfigMediaType: "application/vnd.oci.image.config.v1+json"},
			kind:     constants.ArtifactKindContainerImage, profile: constants.RepositoryProfileContainerImage,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name:     "terraform artifact type",
			metadata: ManifestMetadata{ArtifactType: "application/vnd.cncf.oras.terraform.module.v1"},
			kind:     constants.ArtifactKindTerraformModule, profile: constants.RepositoryProfileTerraform,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name: "cyclonedx referrer",
			metadata: ManifestMetadata{
				ArtifactType: "application/vnd.cyclonedx+json", SubjectDigest: "sha256:subject",
			},
			kind: constants.ArtifactKindSBOMCycloneDX, profile: constants.RepositoryProfileSBOM,
			relationship: constants.ArtifactRelationshipReferrer, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name: "multi-platform image index",
			metadata: ManifestMetadata{
				MediaType:            "application/vnd.oci.image.index.v1+json",
				DescriptorMediaTypes: []string{"application/vnd.oci.image.manifest.v1+json"},
			},
			kind: constants.ArtifactKindImageIndex, profile: constants.RepositoryProfileContainerImage,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceMedium,
		},
		{
			name:     "unknown OCI content",
			metadata: ManifestMetadata{MediaType: "application/vnd.oci.image.manifest.v1+json"},
			kind:     constants.ArtifactKindUnknownOCI, profile: constants.RepositoryProfileGenericOCI,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceLow,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ClassifyManifest(test.metadata)
			if result.Kind != test.kind || result.Profile != test.profile ||
				result.Relationship != test.relationship || result.Confidence != test.confidence {
				t.Fatalf("unexpected classification: %#v", result)
			}
		})
	}
}
