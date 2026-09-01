// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RepositoryCreateModal from './RepositoryCreateModal.vue'

const mocks = vi.hoisted(() => ({
  createRepository: vi.fn(),
  listPolicyPresets: vi.fn(),
}))

vi.mock('../api/projects', () => ({
  createRepository: mocks.createRepository,
  listPolicyPresets: mocks.listPolicyPresets,
  projectKeys: {
    policyPresets: ['registry-policy-presets'],
    repositories: (project: string) => ['projects', project, 'repositories'],
  },
}))

const preset = {
  key: 'protect-production-tags',
  name: 'Protect production tags',
  category: 'Protection',
  description: 'Protect important release references.',
  outcome: 'Production tags remain stable.',
  policy: {
    type: 'tag_protection' as const,
    enabled: true,
    tagPatterns: ['stable', 'prod', 'v*'],
    preventOverwrite: true,
    preventDeletion: true,
    excludeFromLifecycle: true,
    requireReason: false,
  },
}

const retentionPreset = {
  key: 'clean-temporary-builds',
  name: 'Clean temporary builds',
  category: 'Lifecycle',
  description: 'Remove transient images after a short retention window.',
  outcome: 'Temporary tags remain bounded.',
  policy: {
    type: 'retention' as const,
    enabled: true,
    tagPatterns: ['pr-*'],
    expireAfterDays: 14,
    expireAfterDaysEnabled: true,
    keepLast: 5,
    keepLastEnabled: true,
    untaggedGraceDays: 2,
    untaggedGraceDaysEnabled: true,
  },
}

describe('RepositoryCreateModal', () => {
  beforeEach(() => {
    mocks.createRepository.mockReset()
    mocks.listPolicyPresets.mockReset()
    mocks.listPolicyPresets.mockResolvedValue([preset])
  })

  it('previews a preset in an accordion and selects its editable policy', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = mount(RepositoryCreateModal, {
      props: { project: 'payments' },
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })
    await flushPromises()

    const dialog = wrapper.get('dialog')
    expect(dialog.attributes('aria-labelledby')).toBe('create-repository-title')
    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(wrapper.text()).toContain('Protect production tags')
    expect(wrapper.find('#policy-protect-production-tags').isVisible()).toBe(false)

    const toggle = wrapper.get('button[aria-controls="policy-protect-production-tags"]')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('Production tags remain stable.')

    await wrapper.get('input[aria-label="Select Protect production tags"]').setValue(true)
    expect(wrapper.text()).toContain('1 selected')
    expect(wrapper.get('input[placeholder="stable, prod, v*"]').attributes('disabled')).toBeUndefined()
  })

  it('requests dismissal when the shared dialog handles Escape', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = mount(RepositoryCreateModal, {
      props: { project: 'payments' },
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })
    await flushPromises()

    await wrapper.get('dialog').trigger('cancel')

    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('keeps independent retention values while disabling their criteria', async () => {
    mocks.listPolicyPresets.mockResolvedValue([retentionPreset])
    mocks.createRepository.mockResolvedValue({})
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = mount(RepositoryCreateModal, {
      props: { project: 'payments' },
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })
    await flushPromises()

    await wrapper.get('button[aria-controls="policy-clean-temporary-builds"]').trigger('click')
    await wrapper.get('input[aria-label="Select Clean temporary builds"]').setValue(true)
    await wrapper.get('input[aria-label="Enable keep last"]').setValue(false)
    await wrapper.get('input[aria-label="Enable untagged grace days"]').setValue(false)

    const values = wrapper.findAll('input[type="number"]')
    expect(values).toHaveLength(3)
    expect(values[0]!.attributes('disabled')).toBeUndefined()
    expect(values[1]!.attributes('disabled')).toBeDefined()
    expect(values[2]!.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Expire after 14 days')
    expect(wrapper.text()).not.toContain('Keep 5 latest')

    await wrapper.get('input[placeholder="backend or services/api"]').setValue('worker')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.createRepository).toHaveBeenCalledWith('payments', expect.objectContaining({
      name: 'worker',
      policies: [expect.objectContaining({
        expireAfterDays: 14,
        expireAfterDaysEnabled: true,
        keepLast: 5,
        keepLastEnabled: false,
        untaggedGraceDays: 2,
        untaggedGraceDaysEnabled: false,
      })],
    }))
  })
})
