import { apiRequest } from '@/shared/api/client'
import type { RepositorySearchResultPage } from '@/shared/api/models'

export const repositorySearchKeys = {
  list: (query: string, cursor = '') => ['repositories-search', query, cursor] as const,
}

export const searchRepositories = (query = '', cursor = '') => {
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  if (cursor) params.set('cursor', cursor)
  const suffix = params.toString()
  return apiRequest<RepositorySearchResultPage>(`/api/v1/repositories${suffix ? `?${suffix}` : ''}`)
}
