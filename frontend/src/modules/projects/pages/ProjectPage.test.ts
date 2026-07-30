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
  listLifecycleRuns: vi.fn(),
  listTags: vi.fn(),
  listServiceAccounts: vi.fn(),
  listUsers: vi.fn(),
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
  createLifecyclePreview: vi.fn(),
  executeLifecycle: vi.fn(),
  listLifecycleRuns: mocks.listLifecycleRuns,
  registryKeys: {
    inventory: (project: string, repository: string) => ['registry', project, repository, 'inventory'],
    lifecycleRuns: (project: string, repository: string) => ['registry', project, repository, 'lifecycle-runs'],
  },
}))

vi.mock('../api/projects', () => ({
  deleteArtifact: vi.fn(),
  deleteProject: vi.fn(),
  getProject: mocks.getProject,
  listArtifactDeletions: mocks.listArtifactDeletions,
  listMembers: mocks.listMembers,
  listRepositories: mocks.listRepositories,
  listTags: mocks.listTags,
  previewArtifactDeletion: vi.fn(),
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
    mocks.listLifecycleRuns.mockReset()
    mocks.listTags.mockReset()
    mocks.listServiceAccounts.mockReset()
    mocks.listUsers.mockReset()
    mocks.setMember.mockReset()
    mocks.getProject.mockResolvedValue({ id: 'project-1', slug: 'payments', name: 'Payments' })
    mocks.listRepositories.mockResolvedValue([])
    mocks.listMembers.mockResolvedValue([])
    mocks.listArtifactDeletions.mockResolvedValue([])
    mocks.listLifecycleRuns.mockResolvedValue([])
    mocks.listTags.mockResolvedValue({ name: 'payments/api', tags: [] })
    mocks.listServiceAccounts.mockResolvedValue([{
      id: 'service-1', name: 'CI', username: 'ci', description: '', createdAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listUsers.mockResolvedValue([])
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

  it('shows feedback when the pull command cannot be copied', async () => {
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

    expect(wrapper.get('[role="alert"]').text()).toContain('Could not copy the pull command')
    expect(wrapper.text()).not.toContain('Copied')
  })
})
