package application

import (
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
	if metadata.ArtifactType != "" || strings.Contains(strings.ToLower(metadata.MediaType), "artifact") {
		confidence = constants.ClassificationConfidenceMedium
		kind = constants.ArtifactKindGenericOCI
	}
	return ManifestClassification{
		Kind: kind, Profile: constants.RepositoryProfileGenericOCI,
		Relationship: relationship, Source: "media_type", Confidence: confidence,
	}
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
	case strings.Contains(lower, "terraform") || strings.Contains(lower, "opentofu"):
		result.Kind, result.Profile = constants.ArtifactKindTerraformModule, constants.RepositoryProfileTerraform
	case strings.Contains(lower, "helm"):
		result.Kind, result.Profile = constants.ArtifactKindHelmChart, constants.RepositoryProfileGenericOCI
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
