import { apiRequest } from '@/shared/api/client'
import type { CreateUserResponse, User, UserPage } from '@/shared/api/models'

export const userKeys = {
  all: ['users'] as const,
  list: (query: string, cursor = '') => ['users', query, cursor] as const,
}
export const listUsers = (query = '', cursor = '') => {
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  if (cursor) params.set('cursor', cursor)
  const suffix = params.toString()
  return apiRequest<UserPage>(`/api/v1/users${suffix ? `?${suffix}` : ''}`)
}
export const createUser = (input: {
  email: string
  username: string
}) => apiRequest<CreateUserResponse>('/api/v1/users', { method: 'POST', body: JSON.stringify(input) })

export const promoteUserToSystemAdmin = (userId: string) =>
  apiRequest<User>(`/api/v1/users/${encodeURIComponent(userId)}/administrator`, { method: 'PUT' })

export const promoteUserToSystemViewer = (userId: string) =>
  apiRequest<User>(`/api/v1/users/${encodeURIComponent(userId)}/viewer`, { method: 'PUT' })

export const createUserPasswordResetLink = (userId: string) =>
  apiRequest<{ url: string; expiresAt: string }>(`/api/v1/users/${encodeURIComponent(userId)}/password-reset-link`, {
    method: 'POST',
  })

export const disableUser = (userId: string) =>
  apiRequest<void>(`/api/v1/users/${encodeURIComponent(userId)}`, { method: 'DELETE' })

export const updateUser = (userId: string, input: { email?: string; username?: string }) =>
  apiRequest<User>(`/api/v1/users/${encodeURIComponent(userId)}`, { method: 'PATCH', body: JSON.stringify(input) })

export const reactivateUser = (userId: string) =>
  apiRequest<User>(`/api/v1/users/${encodeURIComponent(userId)}/reactivate`, { method: 'POST' })
