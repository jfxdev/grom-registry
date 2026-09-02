// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RepositoryCreateModal from './RepositoryCreateModal.vue'

const mocks = vi.hoisted(() => ({
  createRepository: vi.fn(),
}))

vi.mock('../api/projects', () => ({
  createRepository: mocks.createRepository,
  projectKeys: {
    repositories: (project: string) => ['projects', project, 'repositories'],
  },
}))

function mountModal() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return mount(RepositoryCreateModal, {
    props: { project: 'docker' },
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

describe('RepositoryCreateModal', () => {
  beforeEach(() => {
    mocks.createRepository.mockReset()
  })

  it('creates an empty-policy repository and shows nested-path examples', async () => {
    mocks.createRepository.mockResolvedValue({})
    const wrapper = mountModal()

    expect(wrapper.text()).toContain('Policies can be configured after the repository is created.')
    expect(wrapper.text()).not.toContain('Behavior policies')
    expect(wrapper.text()).toContain('base-images/forgejo')
    expect(wrapper.text()).toContain('services/api')
    expect(wrapper.findAll('.path-examples li')).toHaveLength(3)

    await wrapper.get('input[placeholder="backend or services/api"]').setValue('base-images/forgejo')
    await wrapper.get('input[placeholder="Primary API image"]').setValue('Forgejo base image')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.createRepository).toHaveBeenCalledWith('docker', {
      name: 'base-images/forgejo',
      description: 'Forgejo base image',
      policies: [],
    })
    expect(wrapper.emitted('created')).toHaveLength(1)
  })

  it('requests dismissal when the shared dialog handles Escape', async () => {
    const wrapper = mountModal()

    await wrapper.get('dialog').trigger('cancel')

    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
