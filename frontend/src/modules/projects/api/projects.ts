import { apiRequest } from '@/shared/api/client'
import type {
  CreateRepositoryRequest,
  ArtifactDeletion,
  ArtifactDeletionPage,
  ArtifactDeletionPreview,
  ArtifactDeletionRequest,
  MembershipPage,
  PolicyPreset,
  Project,
  ProjectPage,
  ProjectRole,
  PrincipalKind,
  Repository,
  RepositoryPage,
  RepositoryPolicySet,
  ReplaceRepositoryPoliciesRequest,
  TagPage,
} from '@/shared/api/models'

export const projectKeys = {
  all: ['projects'] as const,
  detail: (slug: string) => ['projects', slug] as const,
  members: (slug: string) => ['projects', slug, 'members'] as const,
  repositories: (slug: string) => ['projects', slug, 'repositories'] as const,
  tags: (slug: string, repository: string) => ['projects', slug, 'repositories', repository, 'tags'] as const,
  policyPresets: ['registry-policy-presets'] as const,
  policies: (slug: string, repositoryId: string) =>
    ['projects', slug, 'repositories', repositoryId, 'policies'] as const,
  artifactDeletions: (slug: string, repository: string) =>
    ['projects', slug, 'repositories', repository, 'artifact-deletions'] as const,
}

export const listProjects = (cursor = '') => apiRequest<ProjectPage>(`/api/v1/projects${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`)
export const getProject = (slug: string) => apiRequest<Project>(`/api/v1/projects/${encodeURIComponent(slug)}`)
export const createProject = (input: { name: string; slug: string }) =>
  apiRequest<Project>('/api/v1/projects', { method: 'POST', body: JSON.stringify(input) })
export const deleteProject = (slug: string) =>
  apiRequest<void>(`/api/v1/projects/${encodeURIComponent(slug)}`, { method: 'DELETE' })
export const listMembers = (slug: string, cursor = '', query = '') => {
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  if (cursor) params.set('cursor', cursor)
  const suffix = params.toString()
  return apiRequest<MembershipPage>(`/api/v1/projects/${encodeURIComponent(slug)}/members${suffix ? `?${suffix}` : ''}`)
}
export const setMember = (slug: string, kind: PrincipalKind, id: string, role: ProjectRole) =>
  apiRequest<{ status: string }>(`/api/v1/projects/${encodeURIComponent(slug)}/members/${kind}/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ role }),
  })
export const deleteMember = (slug: string, kind: PrincipalKind, id: string) =>
  apiRequest<void>(`/api/v1/projects/${encodeURIComponent(slug)}/members/${kind}/${id}`, {
    method: 'DELETE',
  })
export const listRepositories = (slug: string, query = '', cursor = '') => {
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  if (cursor) params.set('cursor', cursor)
  const suffix = params.toString()
  return apiRequest<RepositoryPage>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories${suffix ? `?${suffix}` : ''}`)
}
export const getRepository = (slug: string, repositoryId: string) =>
  apiRequest<Repository>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories/${encodeURIComponent(repositoryId)}`)
export const createRepository = (slug: string, input: CreateRepositoryRequest) =>
  apiRequest<Repository>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
export const archiveRepository = (slug: string, repositoryId: string) =>
  apiRequest<void>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories/${encodeURIComponent(repositoryId)}/archive`, {
    method: 'POST',
  })
export const unarchiveRepository = (slug: string, repositoryId: string) =>
  apiRequest<void>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories/${encodeURIComponent(repositoryId)}/archive`, {
    method: 'DELETE',
  })
export const removeRepository = (slug: string, repositoryId: string) =>
  apiRequest<void>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories/${encodeURIComponent(repositoryId)}`, {
    method: 'DELETE',
  })
export const listPolicyPresets = () =>
  apiRequest<PolicyPreset[]>('/api/v1/registry-policy-presets')
export const getRepositoryPolicies = (slug: string, repositoryId: string) =>
  apiRequest<RepositoryPolicySet>(
    `/api/v1/projects/${encodeURIComponent(slug)}/repositories/${encodeURIComponent(repositoryId)}/policies`,
  )
export const replaceRepositoryPolicies = (
  slug: string,
  repositoryId: string,
  input: ReplaceRepositoryPoliciesRequest,
) =>
  apiRequest<RepositoryPolicySet>(
    `/api/v1/projects/${encodeURIComponent(slug)}/repositories/${encodeURIComponent(repositoryId)}/policies`,
    { method: 'PUT', body: JSON.stringify(input) },
  )
export const listTags = (slug: string, repository: string, query = '', cursor = '') => {
  const params = new URLSearchParams({ repository })
  if (query) params.set('q', query)
  if (cursor) params.set('cursor', cursor)
  return apiRequest<TagPage>(`/api/v1/projects/${encodeURIComponent(slug)}/repository-tags?${params.toString()}`)
}
export const previewArtifactDeletion = (slug: string, input: ArtifactDeletionRequest) =>
  apiRequest<ArtifactDeletionPreview>(`/api/v1/projects/${encodeURIComponent(slug)}/artifact-deletion-previews`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
export const deleteArtifact = (slug: string, input: ArtifactDeletionRequest) =>
  apiRequest<ArtifactDeletion>(`/api/v1/projects/${encodeURIComponent(slug)}/artifact-deletions`, {
    method: 'POST',
    body: JSON.stringify(input),
  }, [500])
export const listArtifactDeletions = (slug: string, repository: string, cursor = '') =>
  apiRequest<ArtifactDeletionPage>(
    `/api/v1/projects/${encodeURIComponent(slug)}/artifact-deletions?repository=${encodeURIComponent(repository)}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`,
  )
