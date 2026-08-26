import { apiRequest } from '@/shared/api/client'
import { auditActionValues, auditResourceKindValues } from '@/shared/api/generated/schema'
import type { AuditAction, AuditEventPage, AuditResourceKind } from '@/shared/api/models'

export type AuditFilters = {
  action?: AuditAction
  resource?: AuditResourceKind
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

export const AUDIT_ACTIONS = auditActionValues
export const AUDIT_RESOURCE_KINDS = auditResourceKindValues
