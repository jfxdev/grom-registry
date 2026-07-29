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

describe('ProjectsPage creation error', () => {
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
})
