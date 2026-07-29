import { apiRequest } from '@/shared/api/client'
import type { Integration } from '@/shared/api/models'

export const integrationKeys = { all: ['integrations'] as const }
export const listIntegrations = () => apiRequest<Integration[]>('/api/v1/integrations')
