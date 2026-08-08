import { apiRequest } from '@/shared/api/client'
import type { APITokenPage, CreatedToken, ServiceAccount, ServiceAccountPage } from '@/shared/api/models'

export const serviceAccountKeys = {
  all: ['service-accounts'] as const,
  list: (includeDisabled = false) => ['service-accounts', includeDisabled ? 'all' : 'active'] as const,
  tokens: (serviceAccountId: string) => ['service-accounts', serviceAccountId, 'tokens'] as const,
}

export const listServiceAccounts = (includeDisabled = false, cursor = '') =>
  apiRequest<ServiceAccountPage>(`/api/v1/service-accounts?includeDisabled=${includeDisabled}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`)
export const createServiceAccount = (input: { name: string; username: string; description: string }) =>
  apiRequest<ServiceAccount>('/api/v1/service-accounts', { method: 'POST', body: JSON.stringify(input) })
export const disableServiceAccount = (id: string) =>
  apiRequest<void>(`/api/v1/service-accounts/${id}`, { method: 'DELETE' })
export const listServiceAccountTokens = (serviceAccountId: string, cursor = '') =>
  apiRequest<APITokenPage>(`/api/v1/service-accounts/${serviceAccountId}/tokens${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`)
export const createServiceAccountToken = (serviceAccountId: string, input: { name: string; expiresAt?: string | null }) =>
  apiRequest<CreatedToken>(`/api/v1/service-accounts/${serviceAccountId}/tokens`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
export const revokeServiceAccountToken = (serviceAccountId: string, tokenId: string) =>
  apiRequest<void>(`/api/v1/service-accounts/${serviceAccountId}/tokens/${tokenId}`, { method: 'DELETE' })
