// @vitest-environment jsdom

import { APIError } from '@/shared/api/client'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProjectsPage from './ProjectsPage.vue'

const mocks = vi.hoisted(() => ({
  createProject: vi.fn(),
  listProjects: vi.fn(),
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({ user: { systemAdmin: true } }),
}))

vi.mock('../api/projects', () => ({
  createProject: mocks.createProject,
  listProjects: mocks.listProjects,
  projectKeys: { all: ['projects'] },
}))

function mountPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return mount(ProjectsPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
}

describe('ProjectsPage', () => {
  beforeEach(() => {
    mocks.createProject.mockReset()
    mocks.listProjects.mockReset()
    mocks.listProjects.mockResolvedValue([])
  })

  it('clears a previous failure when reopening the creation modal', async () => {
    mocks.createProject.mockRejectedValue(new APIError(409, 'conflict', 'Project already exists'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button').trigger('click')
    await wrapper.get('input').setValue('Payments')
    await wrapper.findAll('input')[1]!.setValue('payments')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Project already exists')

    await wrapper.get('button[aria-label="Close"]').trigger('click')
    await wrapper.get('button').trigger('click')

    expect(wrapper.text()).not.toContain('Project already exists')
  })

  it('auto-fills the slug from the name while the slug is untouched', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button').trigger('click')
    const [nameInput, slugInput] = wrapper.get('form').findAll('input')
    await nameInput!.setValue('My Payments App')

    expect((slugInput!.element as HTMLInputElement).value).toBe('my-payments-app')
  })

  it('stops auto-filling the slug once the user edits it, keeping the manual value', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button').trigger('click')
    const [nameInput, slugInput] = wrapper.get('form').findAll('input')
    await nameInput!.setValue('Payments')
    expect((slugInput!.element as HTMLInputElement).value).toBe('payments')

    await slugInput!.setValue('billing')
    await nameInput!.setValue('Payments API')

    expect((slugInput!.element as HTMLInputElement).value).toBe('billing')
  })

  it('resumes auto-filling the slug after the user clears it', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button').trigger('click')
    const [nameInput, slugInput] = wrapper.get('form').findAll('input')
    await nameInput!.setValue('Payments')
    await slugInput!.setValue('billing')
    await slugInput!.setValue('')
    await nameInput!.setValue('Web Store')

    expect((slugInput!.element as HTMLInputElement).value).toBe('web-store')
  })

  it('sanitizes a manually edited slug on blur', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button').trigger('click')
    const slugInput = wrapper.get('form').findAll('input')[1]!
    await slugInput.setValue('My Custom Slug!')
    await slugInput.trigger('blur')

    expect((slugInput.element as HTMLInputElement).value).toBe('my-custom-slug')
  })

  it('filters projects by name or slug from the main panel search', async () => {
    mocks.listProjects.mockResolvedValue([
      {
        id: 'project-1',
        name: 'Payments API',
        slug: 'payments',
        createdBy: 'user-1',
        createdAt: '2026-07-29T10:00:00Z',
      },
      {
        id: 'project-2',
        name: 'Customer Portal',
        slug: 'web-store',
        createdBy: 'user-1',
        createdAt: '2026-07-29T10:00:00Z',
      },
    ])
    const wrapper = mountPage()
    await flushPromises()

    const search = wrapper.get('input[aria-label="Search projects"]')
    await search.setValue('payments')
    expect(wrapper.text()).toContain('Payments API')
    expect(wrapper.text()).not.toContain('Customer Portal')
    expect(wrapper.text()).toContain('1 of 2 on this page')

    await search.setValue('WEB-STORE')
    expect(wrapper.text()).toContain('Customer Portal')
    expect(wrapper.text()).not.toContain('Payments API')
  })

  it('shows a dedicated empty state when the search has no matches', async () => {
    mocks.listProjects.mockResolvedValue([
      {
        id: 'project-1',
        name: 'Payments API',
        slug: 'payments',
        createdBy: 'user-1',
        createdAt: '2026-07-29T10:00:00Z',
      },
    ])
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('input[aria-label="Search projects"]').setValue('missing')

    expect(wrapper.text()).toContain('No matching projects')
    expect(wrapper.text()).toContain('0 of 1 on this page')
  })

  it('separates the pagination footer from the project list', async () => {
    mocks.listProjects.mockResolvedValue([{ id: 'project-1', name: 'Payments', slug: 'payments', createdBy: 'user-1', createdAt: '2026-07-29T10:00:00Z' }])
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.find('.panel-pagination [aria-label="Pagination"]').exists()).toBe(true)
    expect(wrapper.find('.project-list + .panel-pagination').exists()).toBe(true)
    expect(wrapper.find('.project-list .panel-pagination').exists()).toBe(false)
  })

  it('renders ready, pending, stale, and unavailable accounting without treating them as zero', async () => {
    mocks.listProjects.mockResolvedValue([
      { id: 'ready', name: 'Ready', slug: 'ready', createdBy: 'user-1', createdAt: '2026-07-29T10:00:00Z', accountedUsage: { status: 'ready', accountedBytes: 0 } },
      { id: 'pending', name: 'Pending', slug: 'pending', createdBy: 'user-1', createdAt: '2026-07-29T10:00:00Z', accountedUsage: { status: 'pending' } },
      { id: 'stale', name: 'Stale', slug: 'stale', createdBy: 'user-1', createdAt: '2026-07-29T10:00:00Z', accountedUsage: { status: 'stale', accountedBytes: 1024 } },
      { id: 'unavailable', name: 'Unavailable', slug: 'unavailable', createdBy: 'user-1', createdAt: '2026-07-29T10:00:00Z', accountedUsage: { status: 'unavailable' } },
    ])
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('0 B accounted')
    expect(wrapper.text()).toContain('Accounting pending')
    expect(wrapper.text()).toContain('1 KB (stale)')
    expect(wrapper.text()).toContain('Accounting unavailable')
  })
})
