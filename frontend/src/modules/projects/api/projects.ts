import { apiRequest } from '@/shared/api/client'
import type {
  CreateRepositoryRequest,
  ArtifactDeletion,
  ArtifactDeletionPreview,
  ArtifactDeletionRequest,
  Membership,
  PolicyPreset,
  Project,
  ProjectRole,
  PrincipalKind,
  Repository,
  RepositoryPolicySet,
  ReplaceRepositoryPoliciesRequest,
  TagList,
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

export const listProjects = () => apiRequest<Project[]>('/api/v1/projects')
export const getProject = (slug: string) => apiRequest<Project>(`/api/v1/projects/${encodeURIComponent(slug)}`)
export const createProject = (input: { name: string; slug: string }) =>
  apiRequest<Project>('/api/v1/projects', { method: 'POST', body: JSON.stringify(input) })
export const deleteProject = (slug: string) =>
  apiRequest<void>(`/api/v1/projects/${encodeURIComponent(slug)}`, { method: 'DELETE' })
export const listMembers = (slug: string) =>
  apiRequest<Membership[]>(`/api/v1/projects/${encodeURIComponent(slug)}/members`)
export const setMember = (slug: string, kind: PrincipalKind, id: string, role: ProjectRole) =>
  apiRequest<{ status: string }>(`/api/v1/projects/${encodeURIComponent(slug)}/members/${kind}/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ role }),
  })
export const listRepositories = (slug: string) =>
  apiRequest<Repository[]>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories`)
export const createRepository = (slug: string, input: CreateRepositoryRequest) =>
  apiRequest<Repository>(`/api/v1/projects/${encodeURIComponent(slug)}/repositories`, {
    method: 'POST',
    body: JSON.stringify(input),
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
export const listTags = (slug: string, repository: string) =>
  apiRequest<TagList>(`/api/v1/projects/${encodeURIComponent(slug)}/repository-tags?repository=${encodeURIComponent(repository)}`)
export const previewArtifactDeletion = (slug: string, input: ArtifactDeletionRequest) =>
  apiRequest<ArtifactDeletionPreview>(`/api/v1/projects/${encodeURIComponent(slug)}/artifact-deletion-previews`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
export const deleteArtifact = (slug: string, input: ArtifactDeletionRequest) =>
  apiRequest<ArtifactDeletion>(`/api/v1/projects/${encodeURIComponent(slug)}/artifact-deletions`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
export const listArtifactDeletions = (slug: string, repository: string) =>
  apiRequest<ArtifactDeletion[]>(
    `/api/v1/projects/${encodeURIComponent(slug)}/artifact-deletions?repository=${encodeURIComponent(repository)}`,
  )
