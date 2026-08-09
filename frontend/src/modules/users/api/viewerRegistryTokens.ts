import { apiRequest } from '@/shared/api/client'
import type { CreatedViewerRegistryToken, ViewerRegistryToken } from '@/shared/api/models'

export const viewerRegistryTokenKeys = { all: ['viewer-registry-tokens'] as const }

export const listViewerRegistryTokens = () => apiRequest<ViewerRegistryToken[]>('/api/v1/me/registry-tokens')

export const createViewerRegistryToken = (input: { name: string }) =>
  apiRequest<CreatedViewerRegistryToken>('/api/v1/me/registry-tokens', { method: 'POST', body: JSON.stringify(input) })

export const revokeViewerRegistryToken = (tokenId: string) =>
  apiRequest<void>(`/api/v1/me/registry-tokens/${encodeURIComponent(tokenId)}`, { method: 'DELETE' })
