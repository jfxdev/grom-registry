import { apiRequest } from '@/shared/api/client'
import type { User } from '@/shared/api/models'

export const userKeys = { all: ['users'] as const }
export const listUsers = () => apiRequest<User[]>('/api/v1/users')
export const createUser = (input: {
  email: string
  username: string
  password: string
  systemAdmin: boolean
}) => apiRequest<User>('/api/v1/users', { method: 'POST', body: JSON.stringify(input) })

export const createUserPasswordResetLink = (userId: string) =>
  apiRequest<{ url: string; expiresAt: string }>(`/api/v1/users/${encodeURIComponent(userId)}/password-reset-link`, {
    method: 'POST',
  })
