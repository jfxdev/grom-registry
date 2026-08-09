// @vitest-environment jsdom

import { APIError } from '@/shared/api/client'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/vue'
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
	getRepository: vi.fn(),
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
  repositoryId: '',
  routerPush: vi.fn(),
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
	getRepository: mocks.getRepository,
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
  useRoute: () => ({ params: { project: 'payments', repositoryId: mocks.repositoryId || undefined } }),
  useRouter: () => ({ push: mocks.routerPush }),
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

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(ProjectPage, {
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
    cleanup()
    vi.unstubAllGlobals()
  })

  beforeEach(() => {
    mocks.sessionUser.systemAdmin = true
    mocks.getProject.mockReset()
		mocks.getRepository.mockReset()
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
    mocks.routerPush.mockReset()
    mocks.repositoryId = ''
    mocks.getProject.mockResolvedValue({ id: 'project-1', slug: 'payments', name: 'Payments' })
		mocks.getRepository.mockResolvedValue(undefined)
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

  it('hides project settings from non-managers', async () => {
    mocks.sessionUser.systemAdmin = false
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.find('button[aria-label="Project settings"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Add service account')
  })

  it('uses the labelled delete button for project deletion', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Project settings"]').trigger('click')
    await buttonWithText(wrapper, 'Danger zone').trigger('click')
    const deleteButton = buttonWithText(wrapper, 'Delete project')

    expect(deleteButton.classes()).toContain('grom-button-variant-delete')
    expect(deleteButton.get('.lucide-trash-2')).toBeTruthy()
  })

  it('shows API errors in the member dialog', async () => {
    mocks.setMember.mockRejectedValue(new APIError(403, 'forbidden', 'Member assignment denied'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Project settings"]').trigger('click')
    await buttonWithText(wrapper, 'Add member').trigger('click')
    const memberForm = wrapper.get('form[aria-labelledby="add-member-title"]')
    await memberForm.findAll('select')[1]!.setValue('service-1')
    await memberForm.trigger('submit')
    await flushPromises()

    expect(mocks.setMember).toHaveBeenCalledWith('payments', 'service_account', 'service-1', 'reader')
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
    mocks.repositoryId = 'repository-1'
    const wrapper = mountPage()
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
    mocks.repositoryId = 'repository-1'

    renderPage()

    expect(await screen.findByText(/Different primary artifact types/)).toBeTruthy()
    expect(await screen.findByText('Deletion history')).toBeTruthy()
    expect(await screen.findByText(/No reason/)).toBeTruthy()
    await fireEvent.click(await screen.findByRole('button', { name: /sha256:abc/ }))
    expect(screen.getByRole('dialog', { name: 'Manifest details' })).toBeTruthy()
    expect(screen.getByText('Untagged')).toBeTruthy()
    expect(screen.getByText('sha256:subject')).toBeTruthy()
  })

  it('loads a routed repository that is outside the current list page', async () => {
    mocks.repositoryId = 'repository-2'
    mocks.getRepository.mockResolvedValue({
      id: 'repository-2', projectId: 'project-1', name: 'worker', description: '', status: 'active',
      creationSource: 'push', profile: 'unknown', profileSource: 'none', profileConfidence: 'none',
      profileNeedsReview: false, policyVersion: 0, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    })
    const wrapper = mountPage()
    await flushPromises()

    expect(mocks.getRepository).toHaveBeenCalledWith('payments', 'repository-2')
    expect(wrapper.get('#repository-details-title').text()).toBe('worker')
  })

  it('archives an active repository and removes an archived repository', async () => {
    const activeRepository = {
      id: 'repository-1', projectId: 'project-1', name: 'api', description: '', status: 'active',
      creationSource: 'manual', profile: 'unknown', profileSource: 'none', profileConfidence: 'none',
      profileNeedsReview: false, policyVersion: 0, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    }
    mocks.listRepositories.mockResolvedValue([activeRepository])
    mocks.repositoryId = 'repository-1'
    renderPage()
    await fireEvent.click(await screen.findByRole('button', { name: 'Archive' }))
    await waitFor(() => expect(mocks.archiveRepository).toHaveBeenCalledWith('payments', 'repository-1'))

    mocks.listRepositories.mockResolvedValue([{ ...activeRepository, status: 'archived' }])
    cleanup()
    renderPage()
    await fireEvent.click(await screen.findByRole('button', { name: 'Remove logical record' }))
    const dialog = screen.getByRole('dialog', { name: 'Remove logical repository' })
    await fireEvent.submit(within(dialog).getByRole('button', { name: 'Remove record' }).closest('form')!)
    await waitFor(() => expect(mocks.removeRepository).toHaveBeenCalledWith('payments', 'repository-1'))
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
    mocks.repositoryId = 'repository-1'
    renderPage()
    await fireEvent.click(await screen.findByRole('button', { name: 'Review lifecycle' }))
    await waitFor(() => expect(mocks.createLifecyclePreview).toHaveBeenCalledWith('payments', 'api'))
    expect(await screen.findByText(/sha256:blocked/)).toBeTruthy()
    expect(screen.getAllByText('Untagged')).not.toHaveLength(0)
    const dialog = screen.getByText('Lifecycle dry-run').closest('dialog')!
    await fireEvent.update(within(dialog).getByLabelText('Execution reason'), 'retention')
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Delete 1 eligible' }))
    await waitFor(() => expect(mocks.executeLifecycle).toHaveBeenCalledWith('payments', 'preview-1', 'retention'))
    expect(await screen.findByText(/1 deleted,/)).toBeTruthy()
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
    mocks.repositoryId = 'repository-1'
    renderPage()
    await fireEvent.click(await screen.findByRole('button', { name: 'Delete stable' }))
    expect(await screen.findByText('The manifest has referrers')).toBeTruthy()
    expect(screen.getByText('sha256:signature')).toBeTruthy()
    expect(screen.getByText('Untagged manifest')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Delete artifact' }).hasAttribute('disabled')).toBe(true)
  })

  it('opens a repository in its dedicated route', async () => {
    mocks.listRepositories.mockResolvedValue([{
      id: 'repository-1', projectId: 'project-1', name: 'api', description: '', status: 'active',
      creationSource: 'manual', profile: 'unknown', profileSource: 'none', profileConfidence: 'none',
      profileNeedsReview: false, policyVersion: 0, policies: [], createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z',
    }])
    const wrapper = mountPage()
    await flushPromises()

    await buttonWithText(wrapper, 'payments/api').trigger('click')

    expect(mocks.routerPush).toHaveBeenCalledWith({
      name: 'repository-detail',
      params: { project: 'payments', repositoryId: 'repository-1' },
    })
  })

  it('manages existing members and surfaces removal errors', async () => {
    mocks.listMembers.mockResolvedValue([{
      principalKind: 'user', principalId: 'user-2', role: 'writer', createdAt: '2026-07-29T00:00:00Z',
    }])
    mocks.listUsers.mockResolvedValue([{ id: 'user-2', email: 'user@example.com', username: 'writer', systemAdmin: false, createdAt: '2026-07-29T00:00:00Z' }])
    mocks.deleteMember.mockRejectedValue(new APIError(409, 'conflict', 'Member removal denied'))
    renderPage()
    await fireEvent.click(await screen.findByRole('button', { name: 'Project settings' }))
    await fireEvent.click(screen.getByRole('button', { name: 'Change role' }))
    const memberDialog = screen.getByRole('dialog', { name: 'Add service account' })
    await fireEvent.update(within(memberDialog).getByLabelText('Role'), 'admin')
    await fireEvent.submit(within(memberDialog).getByRole('button', { name: 'Add member' }).closest('form')!)
    await waitFor(() => expect(mocks.setMember).toHaveBeenCalledWith('payments', 'user', 'user-2', 'admin'))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Add service account' })).toBeNull())
    await fireEvent.click(screen.getByRole('button', { name: 'Remove user member' }))
    const removalDialog = screen.getByRole('dialog', { name: 'Remove member' })
    await fireEvent.submit(within(removalDialog).getByRole('button', { name: 'Remove member' }).closest('form')!)
    await waitFor(() => expect(mocks.deleteMember).toHaveBeenCalledWith('payments', 'user', 'user-2'))
    expect((await screen.findByText(/Member removal denied/)).textContent).toContain('Member removal denied')
  })

  it('uses a human-readable removal label for service accounts', async () => {
    mocks.listMembers.mockResolvedValue([{
      principalKind: 'service_account', principalId: 'service-1', role: 'writer', createdAt: '2026-07-29T00:00:00Z',
    }])
    renderPage()

    await fireEvent.click(await screen.findByRole('button', { name: 'Project settings' }))

    expect(screen.getByRole('button', { name: 'Remove service account member' })).toBeTruthy()
  })

  it('shows a project-deletion error without leaving the confirmation dialog', async () => {
    mocks.deleteProject.mockRejectedValue(new APIError(409, 'project_not_empty', 'Project still contains repositories'))
    renderPage()
    await fireEvent.click(await screen.findByRole('button', { name: 'Project settings' }))
    await fireEvent.click(await screen.findByRole('button', { name: /Danger zone/ }))
    await fireEvent.click(await screen.findByRole('button', { name: 'Delete project' }))
    const dialog = screen.getByRole('dialog', { name: 'Delete project' })
    await fireEvent.submit(within(dialog).getByRole('button', { name: 'Delete project' }).closest('form')!)

    expect(await screen.findByText('Project still contains repositories')).toBeTruthy()
    expect(screen.getByRole('dialog', { name: 'Delete project' })).toBeTruthy()
  })
})
