import { apiRequest } from '@/shared/api/client'
import type { GarbageCollection, InstallationStatus } from '@/shared/api/models'

export function getInstallationStatus() {
  return apiRequest<InstallationStatus>('/api/v1/settings/status')
}

export function runGarbageCollection() {
  return apiRequest<GarbageCollection>('/api/v1/garbage-collections', { method: 'POST' })
}
