import { apiRequest } from '@/shared/api/client'
import type { APIToken, CreatedToken, ServiceAccount } from '@/shared/api/models'

export const serviceAccountKeys = {
  all: ['service-accounts'] as const,
  tokens: (serviceAccountId: string) => ['service-accounts', serviceAccountId, 'tokens'] as const,
}

export const listServiceAccounts = () => apiRequest<ServiceAccount[]>('/api/v1/service-accounts')
export const createServiceAccount = (input: { name: string; username: string; description: string }) =>
  apiRequest<ServiceAccount>('/api/v1/service-accounts', { method: 'POST', body: JSON.stringify(input) })
export const deleteServiceAccount = (id: string) =>
  apiRequest<void>(`/api/v1/service-accounts/${id}`, { method: 'DELETE' })
export const listServiceAccountTokens = (serviceAccountId: string) =>
  apiRequest<APIToken[]>(`/api/v1/service-accounts/${serviceAccountId}/tokens`)
export const createServiceAccountToken = (serviceAccountId: string, input: { name: string; expiresAt?: string | null }) =>
  apiRequest<CreatedToken>(`/api/v1/service-accounts/${serviceAccountId}/tokens`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
export const revokeServiceAccountToken = (serviceAccountId: string, tokenId: string) =>
  apiRequest<void>(`/api/v1/service-accounts/${serviceAccountId}/tokens/${tokenId}`, { method: 'DELETE' })
