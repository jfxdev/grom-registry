import { apiRequest } from '@/shared/api/client'
import type { Deployment } from '@/shared/api/models'

export function getDeployment() {
  return apiRequest<Deployment>('/api/v1/deployment')
}
