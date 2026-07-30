import { apiRequest } from '@/shared/api/client'
import type { BackupOperation, BackupOverview } from '@/shared/api/models'

export const backupKeys = {
  all: ['backups'] as const,
  overview: (cursor = '') => ['backups', 'overview', cursor] as const,
}

export const getBackupOverview = (cursor = '') => {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''
  return apiRequest<BackupOverview>(`/api/v1/backups${query}`)
}

export const createBackup = () => apiRequest<BackupOperation>('/api/v1/backups', {
  method: 'POST',
})

export const backupDownloadURL = (backupId: string) =>
  `/api/v1/backups/${encodeURIComponent(backupId)}/download`

export const deleteBackup = (backupId: string) =>
  apiRequest<void>(`/api/v1/backups/${encodeURIComponent(backupId)}`, {
    method: 'DELETE',
  })
