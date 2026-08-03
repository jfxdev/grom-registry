// @vitest-environment jsdom

import { APIError } from '@/shared/api/client'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ProjectPage from './ProjectPage.vue'

const mocks = vi.hoisted(() => ({
  sessionUser: {
    id: 'user-1',
    email: 'admin@example.com',
    username: 'admin',
    systemAdmin: true,
    createdAt: '2026-07-29T00:00:00Z',
  },
  getProject: vi.fn(),
  listRepositories: vi.fn(),
  listMembers: vi.fn(),
  listArtifactDeletions: vi.fn(),
  listInventory: vi.fn(),
  listLifecycleRuns: vi.fn(),
  listTags: vi.fn(),
  listServiceAccounts: vi.fn(),
  listUsers: vi.fn(),
  archiveRepository: vi.fn(),
  removeRepository: vi.fn(),
  deleteMember: vi.fn(),
  deleteProject: vi.fn(),
  previewArtifactDeletion: vi.fn(),
  deleteArtifact: vi.fn(),
  createLifecyclePreview: vi.fn(),
  executeLifecycle: vi.fn(),
  setMember: vi.fn(),
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({ user: mocks.sessionUser }),
}))

vi.mock('@/modules/service-accounts/api/serviceAccounts', () => ({
  listServiceAccounts: mocks.listServiceAccounts,
  serviceAccountKeys: {
    all: ['service-accounts'],
    list: (includeDisabled = false) => ['service-accounts', includeDisabled ? 'all' : 'active'],
  },
}))

vi.mock('@/modules/users/api/users', () => ({
  listUsers: mocks.listUsers,
  userKeys: { all: ['users'] },
}))

vi.mock('@/modules/registry', () => ({
  createLifecyclePreview: mocks.createLifecyclePreview,
  executeLifecycle: mocks.executeLifecycle,
  listInventory: mocks.listInventory,
  listLifecycleRuns: mocks.listLifecycleRuns,
  registryKeys: {
    inventory: (project: string, repository: string) => ['registry', project, repository, 'inventory'],
    lifecycleRuns: (project: string, repository: string) => ['registry', project, repository, 'lifecycle-runs'],
  },
}))

vi.mock('../api/projects', () => ({
  archiveRepository: mocks.archiveRepository,
  deleteArtifact: mocks.deleteArtifact,
  deleteMember: mocks.deleteMember,
  deleteProject: mocks.deleteProject,
  getProject: mocks.getProject,
  listArtifactDeletions: mocks.listArtifactDeletions,
  listMembers: mocks.listMembers,
  listRepositories: mocks.listRepositories,
  listTags: mocks.listTags,
  previewArtifactDeletion: mocks.previewArtifactDeletion,
	removeRepository: mocks.removeRepository,
  setMember: mocks.setMember,
  projectKeys: {
    all: ['projects'],
    detail: (slug: string) => ['projects', slug],
    members: (slug: string) => ['projects', slug, 'members'],
    repositories: (slug: string) => ['projects', slug, 'repositories'],
    tags: (slug: string, repository: string) => ['projects', slug, repository, 'tags'],
    artifactDeletions: (slug: string, repository: string) => ['projects', slug, repository, 'deletions'],
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { project: 'payments' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

function mountPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return mount(ProjectPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
}

function buttonWithText(wrapper: ReturnType<typeof mountPage>, text: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text().includes(text))
  if (!button) throw new Error(`Button not found: ${text}`)
  return button
}

describe('ProjectPage membership management', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  beforeEach(() => {
    mocks.sessionUser.systemAdmin = true
    mocks.getProject.mockReset()
    mocks.listRepositories.mockReset()
    mocks.listMembers.mockReset()
    mocks.listArtifactDeletions.mockReset()
    mocks.listInventory.mockReset()
    mocks.listLifecycleRuns.mockReset()
    mocks.listTags.mockReset()
    mocks.listServiceAccounts.mockReset()
    mocks.listUsers.mockReset()
    mocks.setMember.mockReset()
    mocks.archiveRepository.mockReset()
    mocks.removeRepository.mockReset()
    mocks.deleteMember.mockReset()
    mocks.deleteProject.mockReset()
    mocks.previewArtifactDeletion.mockReset()
    mocks.deleteArtifact.mockReset()
    mocks.createLifecyclePreview.mockReset()
    mocks.executeLifecycle.mockReset()
    mocks.getProject.mockResolvedValue({ id: 'project-1', slug: 'payments', name: 'Payments' })
    mocks.listRepositories.mockResolvedValue([])
    mocks.listMembers.mockResolvedValue([])
    mocks.listArtifactDeletions.mockResolvedValue([])
    mocks.listInventory.mockResolvedValue([])
    mocks.listLifecycleRuns.mockResolvedValue([])
    mocks.listTags.mockResolvedValue({ name: 'payments/api', tags: [] })
    mocks.listServiceAccounts.mockResolvedValue([{
      id: 'service-1', name: 'CI', username: 'ci', description: '', createdAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listUsers.mockResolvedValue([])
    mocks.archiveRepository.mockResolvedValue(undefined)
    mocks.removeRepository.mockResolvedValue(undefined)
    mocks.deleteMember.mockResolvedValue(undefined)
    mocks.deleteProject.mockResolvedValue(undefined)
  })

  it('hides member-add actions from non-managers', async () => {
    mocks.sessionUser.systemAdmin = false
    const wrapper = mountPage()
    await flushPromises()

    await buttonWithText(wrapper, 'Members').trigger('click')

    expect(wrapper.text()).not.toContain('Add service account')
  })

  it('uses the labelled delete button for project deletion', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const deleteButton = buttonWithText(wrapper, 'Delete project')

    expect(deleteButton.classes()).toContain('grom-button-variant-delete')
    expect(deleteButton.get('.lucide-trash-2')).toBeTruthy()
  })

  it('shows API errors in the member dialog', async () => {
    mocks.setMember.mockRejectedValue(new APIError(403, 'forbidden', 'Member assignment denied'))
    const wrapper = mountPage()
    await flushPromises()

    await buttonWithText(wrapper, 'Members').trigger('click')
    await buttonWithText(wrapper, 'Add service account').trigger('click')
    await wrapper.get('form[aria-labelledby="add-member-title"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('Member assignment denied')
  })

  it('shows feedback when a command cannot be copied', async () => {
    vi.stubGlobal('navigator', {})
    mocks.listRepositories.mockResolvedValue([{
      id: 'repository-1',
      projectId: 'project-1',
      name: 'api',
      description: '',
      status: 'active',
      creationSource: 'push',
      profile: 'container_image',
      profileSource: 'inferred',
      profileConfidence: 'high',
      profileNeedsReview: false,
      policyVersion: 0,
      policies: [],
      createdAt: '2026-07-29T00:00:00Z',
      updatedAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listTags.mockResolvedValue({ name: 'payments/api', tags: ['latest'] })
    const wrapper = mountPage()
    await flushPromises()

    await buttonWithText(wrapper, 'payments/api').trigger('click')
    await flushPromises()
    await buttonWithText(wrapper, 'Copy pull').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('Could not copy the command')
    expect(wrapper.text()).not.toContain('Copied')
  })

  it('renders repository history and manifest details for an administrator', async () => {
    mocks.listRepositories.mockResolvedValue([{
      id: 'repository-1', projectId: 'project-1', name: 'api', description: '', status: 'active',
      creationSource: 'push', profile: 'mixed', profileSource: 'inferred', profileConfidence: 'low',
      profileNeedsReview: true, policyVersion: 2, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listTags.mockResolvedValue({ name: 'payments/api', tags: ['stable'] })
    mocks.listInventory.mockResolvedValue([{
      id: 'manifest-1', digest: 'sha256:abc', mediaType: '', artifactType: '', subjectDigest: 'sha256:subject',
      observedKind: 'container_image', artifactRelationship: 'referrer', classificationSource: 'inferred', classificationConfidence: 'high',
      manifestSize: 42, state: 'active', firstSeenAt: '2026-07-29T00:00:00Z', lastSeenAt: '2026-07-30T00:00:00Z', tags: [],
    }])
    mocks.listArtifactDeletions.mockResolvedValue([{
      id: 'deletion-1', repository: 'api', digest: 'sha256:old', affectedTags: [], reason: '', status: 'failed',
      message: 'blocked', startedAt: '2026-07-29T00:00:00Z', completedAt: null,
    }])
    mocks.listLifecycleRuns.mockResolvedValue([{
      id: 'run-1', repository: 'api', previewId: 'preview-1', reason: 'clean up', status: 'failed',
      startedAt: '2026-07-29T00:00:00Z', completedAt: null, items: [],
    }])

    const wrapper = mountPage()
    await flushPromises()
    await buttonWithText(wrapper, 'payments/api').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Different primary artifact types')
    expect(wrapper.text()).toContain('created by push')
    expect(wrapper.text()).toContain('Deletion history')
    expect(wrapper.text()).toContain('No reason')
    await wrapper.get('[role="button"][tabindex="0"]').trigger('click')
    expect(wrapper.text()).toContain('Manifest details')
    expect(wrapper.text()).toContain('Untagged')
    expect(wrapper.text()).toContain('sha256:subject')
  })

  it('archives an active repository and removes an archived repository', async () => {
    const activeRepository = {
      id: 'repository-1', projectId: 'project-1', name: 'api', description: '', status: 'active',
      creationSource: 'manual', profile: 'unknown', profileSource: 'none', profileConfidence: 'none',
      profileNeedsReview: false, policyVersion: 0, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    }
    mocks.listRepositories.mockResolvedValue([activeRepository])
    const wrapper = mountPage()
    await flushPromises()
    await buttonWithText(wrapper, 'payments/api').trigger('click')
    await flushPromises()
    await buttonWithText(wrapper, 'Archive').trigger('click')
    await flushPromises()
    expect(mocks.archiveRepository).toHaveBeenCalledWith('payments', 'repository-1')

    mocks.listRepositories.mockResolvedValue([{ ...activeRepository, status: 'archived' }])
    const archived = mountPage()
    await flushPromises()
    await buttonWithText(archived, 'payments/api').trigger('click')
    await flushPromises()
    await buttonWithText(archived, 'Remove logical record').trigger('click')
    await archived.get('form[aria-labelledby="remove-repository-title"]').trigger('submit')
    await flushPromises()
    expect(mocks.removeRepository).toHaveBeenCalledWith('payments', 'repository-1')
  })

  it('reviews and executes an eligible lifecycle run', async () => {
    mocks.listRepositories.mockResolvedValue([{
      id: 'repository-1', projectId: 'project-1', name: 'api', description: '', status: 'active',
      creationSource: 'manual', profile: 'unknown', profileSource: 'none', profileConfidence: 'none',
      profileNeedsReview: false, policyVersion: 0, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    }])
    mocks.createLifecyclePreview.mockResolvedValue({
      id: 'preview-1', repository: 'api', inventoryAt: '2026-07-29T00:00:00Z', policyVersion: 1, evaluatorVersion: 1,
      eligibleCount: 1, retainedCount: 1, blockedCount: 1,
      items: [
        { id: 'item-1', digest: 'sha256:eligible', decision: 'eligible', tags: ['old'], reasons: ['expired'] },
        { id: 'item-2', digest: 'sha256:retained', decision: 'retained', tags: [], reasons: ['keep'] },
        { id: 'item-3', digest: 'sha256:blocked', decision: 'blocked', tags: [], reasons: ['referrer'] },
      ],
    })
    mocks.executeLifecycle.mockResolvedValue({
      id: 'run-1', repository: 'api', previewId: 'preview-1', reason: 'retention', status: 'completed',
      startedAt: '2026-07-29T00:00:00Z', completedAt: '2026-07-29T00:01:00Z',
      items: [{ id: 'item-1', digest: 'sha256:eligible', status: 'deleted', message: '' }, { id: 'item-2', digest: 'sha256:retained', status: 'skipped', message: 'retained' }],
    })
    const wrapper = mountPage()
    await flushPromises()
    await buttonWithText(wrapper, 'payments/api').trigger('click')
    await flushPromises()
    await buttonWithText(wrapper, 'Review lifecycle').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('sha256:blocked')
    expect(wrapper.text()).toContain('Untagged')
    const reason = wrapper.get('textarea')
    await reason.setValue('retention')
    await buttonWithText(wrapper, 'Delete 1 eligible').trigger('click')
    await flushPromises()
    expect(mocks.executeLifecycle).toHaveBeenCalledWith('payments', 'preview-1', 'retention')
    expect(wrapper.text()).toContain('1 deleted,')
  })

  it('shows blocked artifact-deletion details and requires a policy reason', async () => {
    mocks.listRepositories.mockResolvedValue([{
      id: 'repository-1', projectId: 'project-1', name: 'api', description: '', status: 'active',
      creationSource: 'manual', profile: 'unknown', profileSource: 'none', profileConfidence: 'none',
      profileNeedsReview: false, policyVersion: 0, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listTags.mockResolvedValue({ name: 'payments/api', tags: ['stable'] })
    mocks.previewArtifactDeletion.mockResolvedValue({
      repository: 'api', digest: 'sha256:stable', affectedTags: [], requiresReason: true,
      blockedReasons: ['The manifest has referrers'], relatedArtifacts: ['sha256:signature'],
    })
    const wrapper = mountPage()
    await flushPromises()
    await buttonWithText(wrapper, 'payments/api').trigger('click')
    await flushPromises()
    await wrapper.get('button[aria-label="Delete stable"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('The manifest has referrers')
    expect(wrapper.text()).toContain('sha256:signature')
    expect(wrapper.text()).toContain('Untagged manifest')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })

  it('manages existing members and surfaces removal errors', async () => {
    mocks.listMembers.mockResolvedValue([{
      principalKind: 'user', principalId: 'user-2', role: 'writer', createdAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listUsers.mockResolvedValue([{ id: 'user-2', email: 'user@example.com', username: 'writer', systemAdmin: false, createdAt: '2026-07-29T00:00:00Z' }])
    mocks.deleteMember.mockRejectedValue(new APIError(409, 'conflict', 'Member removal denied'))
    const wrapper = mountPage()
    await flushPromises()
    await buttonWithText(wrapper, 'Members').trigger('click')
    await buttonWithText(wrapper, 'Change role').trigger('click')
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('user')
    await buttonWithText(wrapper, 'Cancel').trigger('click')
    await wrapper.get('button[aria-label="Remove user member"]').trigger('click')
    await wrapper.get('form[aria-labelledby="remove-member-title"]').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Member removal denied')
  })

  it('shows a project-deletion error without leaving the confirmation dialog', async () => {
    mocks.deleteProject.mockRejectedValue(new APIError(409, 'project_not_empty', 'Project still contains repositories'))
    const wrapper = mountPage()
    await flushPromises()
    await buttonWithText(wrapper, 'Delete project').trigger('click')
    await wrapper.get('form[aria-labelledby="delete-project-title"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('.error-text').text()).toContain('Project still contains repositories')
    expect(wrapper.find('form[aria-labelledby="delete-project-title"]').exists()).toBe(true)
  })
})
