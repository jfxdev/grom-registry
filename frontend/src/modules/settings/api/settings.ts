import { apiRequest } from '@/shared/api/client'
import type { InstallationStatus } from '@/shared/api/models'

export function getInstallationStatus() {
  return apiRequest<InstallationStatus>('/api/v1/settings/status')
}
