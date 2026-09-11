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
			name:     "terraform-named artifact type stays a module",
			metadata: ManifestMetadata{ArtifactType: "application/vnd.cncf.oras.terraform.module.v1"},
			kind:     constants.ArtifactKindOpenTofuModule, profile: constants.RepositoryProfileOpenTofu,
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
			kind: constants.ArtifactKindOpenTofuModule, profile: constants.RepositoryProfileOpenTofu,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name: "helm chart config",
			metadata: ManifestMetadata{
				MediaType:       "application/vnd.oci.image.manifest.v1+json",
				ConfigMediaType: "application/vnd.cncf.helm.config.v1+json",
				LayerMediaTypes: []string{"application/vnd.cncf.helm.chart.content.v1.tar+gzip"},
			},
			kind: constants.ArtifactKindHelmChart, profile: constants.RepositoryProfileHelmChart,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name: "wasm component artifact type",
			metadata: ManifestMetadata{
				MediaType:       "application/vnd.oci.image.manifest.v1+json",
				ArtifactType:    "application/vnd.bytecodealliance.component.v1+wasm",
				ConfigMediaType: "application/vnd.oci.empty.v1+json",
			},
			kind: constants.ArtifactKindWASM, profile: constants.RepositoryProfileWASM,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
		},
		{
			name: "wasm module recognised from its layer",
			metadata: ManifestMetadata{
				MediaType:       "application/vnd.oci.image.manifest.v1+json",
				ConfigMediaType: "application/vnd.oci.empty.v1+json",
				LayerMediaTypes: []string{"application/wasm"},
			},
			kind: constants.ArtifactKindWASM, profile: constants.RepositoryProfileWASM,
			relationship: constants.ArtifactRelationshipPrimary, confidence: constants.ClassificationConfidenceHigh,
			source: "layer_media_type",
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

func TestIsFallbackReferrersTag(t *testing.T) {
	subject := "sha256-" + strings.Repeat("a", 64)
	// The bare form is the OCI referrers fallback index ORAS writes when the
	// registry's referrers API is unavailable; the suffixed forms are sigstore's.
	recognised := []string{subject, subject + ".sig", subject + ".att", subject + ".sbom",
		"sha512-" + strings.Repeat("b", 128)}
	for _, tag := range recognised {
		if !IsFallbackReferrersTag(tag) {
			t.Fatalf("expected %q to be recognised as a referrers fallback tag", tag)
		}
	}
	for _, tag := range []string{"latest", "v1.0.0", "sha256-short", "sha256-" + strings.Repeat("a", 64) + ".tar.gz"} {
		if IsFallbackReferrersTag(tag) {
			t.Fatalf("expected %q not to be recognised as a referrers fallback tag", tag)
		}
	}
}

func TestFallbackReferrersTagsDoNotVoteOnTheProfile(t *testing.T) {
	// An ORAS attach against a registry without the referrers API leaves a tagged
	// OCI index behind. It classifies as an image index, so without this guard it
	// would conflict with the repository's real profile and force mixed.
	index := ClassifyManifest(ManifestMetadata{
		MediaType:            "application/vnd.oci.image.index.v1+json",
		DescriptorMediaTypes: []string{"application/vnd.oci.image.manifest.v1+json"},
	})
	fallbackTag := "sha256-" + strings.Repeat("a", 64)
	if votesOnProfile(fallbackTag, index) {
		t.Fatal("a referrers fallback index must not vote on the repository profile")
	}
	if !votesOnProfile("1.0.0", index) {
		t.Fatal("an ordinary tagged primary manifest must still vote")
	}
}
