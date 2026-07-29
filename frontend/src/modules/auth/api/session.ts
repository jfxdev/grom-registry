import { apiRequest } from '@/shared/api/client'
import type { User } from '@/shared/api/models'

export function getCurrentUser() {
  return apiRequest<User>('/api/v1/me')
}

export function createSession(input: { email: string; password: string }) {
  return apiRequest<User>('/api/v1/session', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function deleteSession() {
  return apiRequest<void>('/api/v1/session', { method: 'DELETE' })
}

export function changePassword(input: { currentPassword: string; newPassword: string }) {
  return apiRequest<void>('/api/v1/me/password', {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

export function completePasswordReset(input: { token: string; newPassword: string }) {
  return apiRequest<void>('/api/v1/password-resets', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
