import { apiRequest } from '@/shared/api/client'
import type { AuditEventPage } from '@/shared/api/models'

export type AuditFilters = {
  action?: string
  resource?: string
  actor?: string
  from?: string
  to?: string
}

export const auditEventKeys = {
  all: ['audit-events'] as const,
  list: (filters: AuditFilters, cursor = '') => ['audit-events', filters, cursor] as const,
}

export const listAuditEvents = (filters: AuditFilters = {}, cursor = '') => {
  const params = new URLSearchParams()
  if (filters.action) params.set('action', filters.action)
  if (filters.resource) params.set('resource', filters.resource)
  if (filters.actor) params.set('actor', filters.actor)
  if (filters.from) params.set('from', filters.from)
  if (filters.to) params.set('to', filters.to)
  if (cursor) params.set('cursor', cursor)
  const suffix = params.toString()
  return apiRequest<AuditEventPage>(`/api/v1/audit-events${suffix ? `?${suffix}` : ''}`)
}

// Curated option lists for the filter dropdowns. These mirror the audit action
// and resource-kind constants defined in backend/internal/constants/audit.go.
export const AUDIT_RESOURCE_KINDS = [
  'authentication',
  'user',
  'service_account',
  'project',
  'membership',
  'registry_repository',
  'artifact_deletion',
  'lifecycle_run',
  'backup',
  'garbage_collection',
] as const

export const AUDIT_ACTIONS = [
  'identity.login_succeeded',
  'identity.login_failed',
  'identity.registry_auth_failed',
  'identity.user_created',
  'identity.user_promoted_to_system_admin',
  'identity.user_promoted_to_system_viewer',
  'identity.user_disabled',
  'identity.service_account_created',
  'identity.service_account_disabled',
  'identity.access_key_created',
  'identity.access_key_revoked',
  'identity.user_password_changed',
  'identity.user_password_reset_link_created',
  'identity.user_password_reset_completed',
  'projects.project_created',
  'projects.project_delete_requested',
  'projects.project_deleted',
  'projects.membership_upserted',
  'projects.membership_removed',
  'registry.repository_created_from_push',
  'registry.repository_policies_updated',
  'registry.repository_archived',
  'registry.repository_unarchived',
  'registry.repository_removed',
  'registry.artifact_deletion_started',
  'registry.artifact_deletion_completed',
  'registry.artifact_deletion_failed',
  'registry.lifecycle_preview_created',
  'registry.lifecycle_run_started',
  'registry.lifecycle_item_deleted',
  'registry.lifecycle_item_skipped',
  'registry.lifecycle_item_failed',
  'registry.lifecycle_run_completed',
  'registry.lifecycle_run_failed',
  'platform.restore_completed',
  'platform.backup_created',
  'platform.backup_delete_requested',
  'platform.backup_deleted',
  'platform.garbage_collection_started',
  'platform.garbage_collection_completed',
  'platform.garbage_collection_failed',
] as const
