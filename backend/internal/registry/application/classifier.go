package application

import (
	"regexp"
	"strings"

	"github.com/jfxdev/grom/backend/internal/constants"
)

type ManifestClassification struct {
	Kind         string
	Profile      string
	Relationship string
	Source       string
	Confidence   string
}

func ClassifyManifest(metadata ManifestMetadata) ManifestClassification {
	relationship := constants.ArtifactRelationshipPrimary
	if metadata.SubjectDigest != "" {
		relationship = constants.ArtifactRelationshipReferrer
	}
	classification := classifyByValue(metadata.ArtifactType, "artifact_type", relationship)
	if classification != nil {
		return *classification
	}
	if classification = classifyByValue(metadata.ConfigMediaType, "config_media_type", relationship); classification != nil {
		return *classification
	}
	for _, mediaType := range metadata.LayerMediaTypes {
		if classification = classifyByValue(mediaType, "layer_media_type", relationship); classification != nil {
			return *classification
		}
	}
	imageDescriptors := 0
	for _, mediaType := range metadata.DescriptorMediaTypes {
		lower := strings.ToLower(mediaType)
		if strings.Contains(lower, "image.manifest") || strings.Contains(lower, "manifest.v2") {
			imageDescriptors++
		}
	}
	if imageDescriptors > 0 {
		return ManifestClassification{
			Kind: constants.ArtifactKindImageIndex, Profile: constants.RepositoryProfileContainerImage,
			Relationship: relationship, Source: "index_descriptor",
			Confidence: constants.ClassificationConfidenceMedium,
		}
	}
	confidence := constants.ClassificationConfidenceLow
	kind := constants.ArtifactKindUnknownOCI
	source := "media_type"
	if metadata.ArtifactType != "" {
		// An unrecognised but declared artifactType is still a deliberate
		// statement of intent by the client, so report where the decision
		// actually came from.
		confidence = constants.ClassificationConfidenceMedium
		kind = constants.ArtifactKindGenericOCI
		source = "artifact_type"
	} else if strings.Contains(strings.ToLower(metadata.MediaType), "artifact") {
		confidence = constants.ClassificationConfidenceMedium
		kind = constants.ArtifactKindGenericOCI
	}
	return ManifestClassification{
		Kind: kind, Profile: constants.RepositoryProfileGenericOCI,
		Relationship: relationship, Source: source, Confidence: confidence,
	}
}

// fallbackReferrersTagPattern matches the tag schemes clients fall back to when
// they cannot attach content through the registry's referrers API. ORAS writes a
// bare <algorithm>-<digest> index listing the referrers; sigstore appends .sig,
// .att or .sbom. Both are tagged primary manifests that describe another
// artifact rather than the repository's own content, so neither may vote on the
// repository profile.
var fallbackReferrersTagPattern = regexp.MustCompile(`^sha(?:256|512)-[0-9a-f]{32,128}(?:\.[a-zA-Z0-9]+)?$`)

func IsFallbackReferrersTag(tag string) bool {
	return fallbackReferrersTagPattern.MatchString(tag)
}

func classifyByValue(value, source, relationship string) *ManifestClassification {
	lower := strings.ToLower(value)
	if lower == "" {
		return nil
	}
	result := &ManifestClassification{
		Relationship: relationship, Source: source, Confidence: constants.ClassificationConfidenceHigh,
	}
	switch {
	case strings.Contains(lower, "cyclonedx"):
		result.Kind, result.Profile = constants.ArtifactKindSBOMCycloneDX, constants.RepositoryProfileSBOM
	case strings.Contains(lower, "spdx") || strings.Contains(lower, "sbom"):
		result.Kind, result.Profile = constants.ArtifactKindSBOMSPDX, constants.RepositoryProfileSBOM
	// Both keywords stay matched: OpenTofu is the product language, but
	// terraform-named artifact types exist in the wider ecosystem, such as
	// application/vnd.cncf.oras.terraform.module.v1, and clients push them.
	case strings.Contains(lower, "opentofu") || strings.Contains(lower, "terraform"):
		result.Kind, result.Profile = constants.ArtifactKindOpenTofuModule, constants.RepositoryProfileOpenTofu
	case strings.Contains(lower, "helm"):
		result.Kind, result.Profile = constants.ArtifactKindHelmChart, constants.RepositoryProfileHelmChart
	// Matches application/wasm layers as well as the config and artifact types
	// the WebAssembly toolchains stamp, such as application/vnd.wasm.config.v0+json
	// and application/vnd.bytecodealliance.component.v1+wasm.
	case strings.Contains(lower, "wasm"):
		result.Kind, result.Profile = constants.ArtifactKindWASM, constants.RepositoryProfileWASM
	case strings.Contains(lower, "cosign") || strings.Contains(lower, "notation") ||
		strings.Contains(lower, "notary") || strings.Contains(lower, "signature") ||
		strings.Contains(lower, "sigstore"):
		result.Kind, result.Profile = constants.ArtifactKindSignature, constants.RepositoryProfileGenericOCI
	case strings.Contains(lower, "image.config") || strings.Contains(lower, "container.image") ||
		strings.Contains(lower, "containerimage.config"):
		result.Kind, result.Profile = constants.ArtifactKindContainerImage, constants.RepositoryProfileContainerImage
	default:
		return nil
	}
	return result
}
