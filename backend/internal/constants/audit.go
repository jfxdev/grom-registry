package constants

const (
	AuditUserPasswordChanged          = "identity.user_password_changed"
	AuditUserPasswordResetLinkCreated = "identity.user_password_reset_link_created"
	AuditUserPasswordResetCompleted   = "identity.user_password_reset_completed"
	AuditRestoreCompleted             = "platform.restore_completed"
	AuditBackupCreated                = "platform.backup_created"
	AuditBackupDeleteRequested        = "platform.backup_delete_requested"
	AuditBackupDeleted                = "platform.backup_deleted"

	AuditLifecyclePreviewCreated   = "registry.lifecycle_preview_created"
	AuditLifecycleRunStarted       = "registry.lifecycle_run_started"
	AuditLifecycleItemDeleted      = "registry.lifecycle_item_deleted"
	AuditLifecycleItemSkipped      = "registry.lifecycle_item_skipped"
	AuditLifecycleItemFailed       = "registry.lifecycle_item_failed"
	AuditLifecycleRunCompleted     = "registry.lifecycle_run_completed"
	AuditLifecycleRunFailed        = "registry.lifecycle_run_failed"
	AuditRepositoryCreatedFromPush = "registry.repository_created_from_push"
	AuditRepositoryPoliciesUpdated = "registry.repository_policies_updated"
	AuditArtifactDeletionStarted   = "registry.artifact_deletion_started"
	AuditArtifactDeletionCompleted = "registry.artifact_deletion_completed"
	AuditArtifactDeletionFailed    = "registry.artifact_deletion_failed"

	AuditResourceRepository       = "registry_repository"
	AuditResourceArtifactDeletion = "artifact_deletion"
	AuditResourceLifecycleRun     = "lifecycle_run"
	AuditResourceUser             = "user"
	AuditResourceBackup           = "backup"
)
