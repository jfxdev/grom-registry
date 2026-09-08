import { apiRequest } from '@/shared/api/client'
import type { GarbageCollection, InstallationDiagnostics, InstallationStatus } from '@/shared/api/models'

export function getInstallationStatus() {
  return apiRequest<InstallationStatus>('/api/v1/settings/status')
}

export function getInstallationDiagnostics() {
  return apiRequest<InstallationDiagnostics>('/api/v1/settings/diagnostics')
}

export function runGarbageCollection() {
  return apiRequest<GarbageCollection>('/api/v1/garbage-collections', { method: 'POST' })
}
