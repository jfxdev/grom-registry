import { apiRequest } from '@/shared/api/client'
import type { LifecyclePreview, LifecycleRun, ManifestInventory } from '@/shared/api/models'

export const registryKeys = {
  inventory: (project: string, repository: string) =>
    ['registry', project, repository, 'inventory'] as const,
  lifecycleRuns: (project: string, repository: string) =>
    ['registry', project, repository, 'lifecycle-runs'] as const,
}

export const listInventory = (project: string, repository: string) =>
  apiRequest<ManifestInventory[]>(
    `/api/v1/projects/${encodeURIComponent(project)}/repository-inventory?repository=${encodeURIComponent(repository)}`,
  )

export const reconcileInventory = (project: string, repository: string) =>
  apiRequest<ManifestInventory[]>(
    `/api/v1/projects/${encodeURIComponent(project)}/repository-inventory-reconciliations`,
    { method: 'POST', body: JSON.stringify({ repository }) },
  )

export const createLifecyclePreview = (project: string, repository: string) =>
  apiRequest<LifecyclePreview>(
    `/api/v1/projects/${encodeURIComponent(project)}/lifecycle-previews`,
    { method: 'POST', body: JSON.stringify({ repository }) },
  )

export const executeLifecycle = (project: string, previewId: string, reason: string) =>
  apiRequest<LifecycleRun>(
    `/api/v1/projects/${encodeURIComponent(project)}/lifecycle-runs`,
    { method: 'POST', body: JSON.stringify({ previewId, reason }) },
  )

export const listLifecycleRuns = (project: string, repository: string) =>
  apiRequest<LifecycleRun[]>(
    `/api/v1/projects/${encodeURIComponent(project)}/lifecycle-runs?repository=${encodeURIComponent(repository)}`,
  )
