package application

import (
	"strings"
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
		source       string
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
		{
			name: "opentofu module package",
			metadata: ManifestMetadata{
				MediaType:       "application/vnd.oci.image.manifest.v1+json",
				ArtifactType:    "application/vnd.opentofu.modulepkg",
				ConfigMediaType: "application/vnd.oci.empty.v1+json",
				LayerMediaTypes: []string{"archive/tar+gzip"},
			},
			kind: constants.ArtifactKindTerraformModule, profile: constants.RepositoryProfileTerraform,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name: "unrecognised artifact type stays generic",
			metadata: ManifestMetadata{
				MediaType:       "application/vnd.oci.image.manifest.v1+json",
				ArtifactType:    "application/vnd.acme.bundle.v1+json",
				ConfigMediaType: "application/vnd.oci.empty.v1+json",
			},
			kind: constants.ArtifactKindGenericOCI, profile: constants.RepositoryProfileGenericOCI,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceMedium,
			source: "artifact_type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ClassifyManifest(test.metadata)
			if result.Kind != test.kind || result.Profile != test.profile ||
				result.Relationship != test.relationship || result.Confidence != test.confidence {
				t.Fatalf("unexpected classification: %#v", result)
			}
			if test.source != "" && result.Source != test.source {
				t.Fatalf("expected classification source %q, got %q", test.source, result.Source)
			}
		})
	}
}

func TestIsFallbackSignatureTag(t *testing.T) {
	subject := "sha256-" + strings.Repeat("a", 64)
	for _, tag := range []string{subject + ".sig", subject + ".att", subject + ".sbom"} {
		if !IsFallbackSignatureTag(tag) {
			t.Fatalf("expected %q to be recognised as a sigstore fallback tag", tag)
		}
	}
	for _, tag := range []string{"latest", "v1.0.0", subject, "sha256-short.sig", subject + ".tar"} {
		if IsFallbackSignatureTag(tag) {
			t.Fatalf("expected %q not to be recognised as a sigstore fallback tag", tag)
		}
	}
}
