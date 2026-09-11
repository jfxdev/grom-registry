package constants

const (
	RegistryActionPull   = "pull"
	RegistryActionPush   = "push"
	RegistryActionDelete = "delete"
	RegistryService      = "grom-registry"
	RegistryIssuer       = "grom"

	RepositoryStatusEmpty    = "empty"
	RepositoryStatusActive   = "active"
	RepositoryStatusArchived = "archived"

	RepositoryCreationManual     = "manual"
	RepositoryCreationPush       = "push"
	RepositoryCreationReconciled = "reconciled"

	RepositoryProfileUnknown        = "unknown"
	RepositoryProfileContainerImage = "container_image"
	RepositoryProfileOpenTofu       = "opentofu_module"
	RepositoryProfileHelmChart      = "helm_chart"
	RepositoryProfileWASM           = "wasm"
	RepositoryProfileSBOM           = "sbom"
	RepositoryProfileGenericOCI     = "generic_oci"
	RepositoryProfileMixed          = "mixed"

	ProfileSourceNone     = "none"
	ProfileSourceInferred = "inferred"

	ClassificationConfidenceNone   = "none"
	ClassificationConfidenceLow    = "low"
	ClassificationConfidenceMedium = "medium"
	ClassificationConfidenceHigh   = "high"

	ArtifactKindContainerImage = "container_image"
	ArtifactKindImageIndex     = "image_index"
	ArtifactKindOpenTofuModule = "opentofu_module"
	ArtifactKindHelmChart      = "helm_chart"
	// WebAssembly modules and components share one kind: Grom does not inspect
	// the binary, and the media types in use do not reliably separate them.
	ArtifactKindWASM          = "wasm"
	ArtifactKindSBOMSPDX      = "sbom_spdx"
	ArtifactKindSBOMCycloneDX = "sbom_cyclonedx"
	ArtifactKindSignature     = "signature"
	ArtifactKindGenericOCI    = "generic_oci"
	ArtifactKindUnknownOCI    = "unknown_oci"

	ArtifactRelationshipPrimary  = "primary"
	ArtifactRelationshipReferrer = "referrer"

	RepositoryPolicyTagProtection  = "tag_protection"
	RepositoryPolicyImmutability   = "immutability"
	RepositoryPolicyRetention      = "retention"
	RepositoryPolicyTagNaming      = "tag_naming"
	RepositoryPolicyManualDeletion = "manual_deletion"

	InventoryStateActive   = "active"
	InventoryStateUntagged = "untagged"
	InventoryStateMissing  = "missing"
	InventoryStateDeleted  = "deleted"

	ArtifactDeletionRunning   = "running"
	ArtifactDeletionCompleted = "completed"
	ArtifactDeletionFailed    = "failed"

	LifecycleDecisionEligible = "eligible"
	LifecycleDecisionRetained = "retained"
	LifecycleDecisionBlocked  = "blocked"
	LifecycleEvaluatorVersion = 2

	LifecyclePreviewReady     = "ready"
	LifecyclePreviewExecuting = "executing"
	LifecyclePreviewExecuted  = "executed"
	LifecyclePreviewExpired   = "expired"

	LifecycleRunRunning            = "running"
	LifecycleRunCompleted          = "completed"
	LifecycleRunPartiallyCompleted = "partially_completed"
	LifecycleRunFailed             = "failed"

	LifecycleItemDeleted = "deleted"
	LifecycleItemSkipped = "skipped"
	LifecycleItemFailed  = "failed"
)
