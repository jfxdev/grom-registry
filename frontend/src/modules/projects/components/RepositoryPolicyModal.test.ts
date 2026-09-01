// @vitest-environment jsdom

import type { Repository } from '@/shared/api/models'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import RepositoryPolicyModal from './RepositoryPolicyModal.vue'

const mocks = vi.hoisted(() => ({
  replaceRepositoryPolicies: vi.fn(),
}))

vi.mock('../api/projects', () => ({
  replaceRepositoryPolicies: mocks.replaceRepositoryPolicies,
}))

const repository: Repository = {
  id: '9d7c524a-73ec-4aae-91dd-cd311f147005',
  projectId: '0f75bc96-d0dc-4d25-a2cc-a36238425d7e',
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
  accountedUsage: { status: 'pending' },
  createdAt: '2026-07-27T12:00:00Z',
  updatedAt: '2026-07-27T12:00:00Z',
}

describe('RepositoryPolicyModal', () => {
  beforeAll(() => {
    HTMLElement.prototype.scrollIntoView = () => {}
  })

  beforeEach(() => {
    mocks.replaceRepositoryPolicies.mockReset()
    mocks.replaceRepositoryPolicies.mockResolvedValue({
      repositoryId: repository.id,
      version: 1,
      policies: [],
    })
  })

  it('adds and saves a versioned retention policy', async () => {
    const wrapper = mount(RepositoryPolicyModal, {
      props: { project: 'payments', repository },
    })

    expect(wrapper.get('input[aria-label="Select policy type"]').attributes('aria-label')).toBe('Select policy type')
    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text().includes('Add policy'))!.trigger('click')
    expect(wrapper.text()).toContain('Expire after days')
    expect(wrapper.get('input[aria-label="Enable Retention policy"]').classes()).toContain('retention-toggle')

    await buttons.find((button) => button.text().includes('Save policies'))!.trigger('click')
    await flushPromises()

    expect(mocks.replaceRepositoryPolicies).toHaveBeenCalledWith(
      'payments',
      repository.id,
      expect.objectContaining({
        expectedVersion: 0,
        policies: [
          expect.objectContaining({
            type: 'retention',
            expireAfterDays: 30,
            expireAfterDaysEnabled: true,
            keepLast: 10,
            keepLastEnabled: true,
            untaggedGraceDays: 7,
            untaggedGraceDaysEnabled: true,
          }),
        ],
      }),
    )
    expect(wrapper.emitted('saved')?.[0]?.[0]).toMatchObject({ version: 1 })
  })

  it('persists only the enabled retention criteria while preserving their values', async () => {
    const wrapper = mount(RepositoryPolicyModal, {
      props: { project: 'payments', repository },
    })

    await wrapper.findAll('button').find((button) => button.text().includes('Add policy'))!.trigger('click')
    await wrapper.get('input[aria-label="Enable keep last"]').setValue(false)
    await wrapper.get('input[aria-label="Enable untagged grace days"]').setValue(false)
    const inputs = wrapper.findAll('input[type="number"]')
    await inputs[0]!.setValue('7')

    expect(wrapper.findAll('.retention-criterion-card')).toHaveLength(3)
    expect(inputs[1]!.attributes('disabled')).toBeDefined()
    expect(inputs[2]!.attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('.retention-criterion-card')[1]!.classes()).toContain('disabled')
    expect(wrapper.findAll('.retention-criterion-card')[2]!.classes()).toContain('disabled')

    await wrapper.findAll('button').find((button) => button.text().includes('Save policies'))!.trigger('click')
    await flushPromises()

    expect(mocks.replaceRepositoryPolicies).toHaveBeenCalledWith(
      'payments',
      repository.id,
      expect.objectContaining({
        policies: [expect.objectContaining({
          expireAfterDays: 7,
          expireAfterDaysEnabled: true,
          keepLast: 10,
          keepLastEnabled: false,
          untaggedGraceDays: 7,
          untaggedGraceDaysEnabled: false,
        })],
      }),
    )
  })

  it('clears non-positive and non-integer retention values before saving', async () => {
    const wrapper = mount(RepositoryPolicyModal, {
      props: { project: 'payments', repository },
    })

    await wrapper.findAll('button').find((button) => button.text().includes('Add policy'))!.trigger('click')
    const inputs = wrapper.findAll('input[type="number"]')
    expect(inputs).toHaveLength(3)
    await inputs[0]!.setValue('1.5')
    await inputs[1]!.setValue('0')
    await inputs[2]!.setValue('-1')
    await wrapper.findAll('button').find((button) => button.text().includes('Save policies'))!.trigger('click')
    await flushPromises()

    expect(mocks.replaceRepositoryPolicies).toHaveBeenCalledWith(
      'payments',
      repository.id,
      expect.objectContaining({
        policies: [
          expect.objectContaining({
            expireAfterDays: undefined,
            keepLast: undefined,
            untaggedGraceDays: undefined,
          }),
        ],
      }),
    )
  })

  it('keeps the policy selector menu in the dialog layer', async () => {
    const wrapper = mount(RepositoryPolicyModal, {
      attachTo: document.body,
      props: { project: 'payments', repository },
    })

    try {
      await wrapper.vm.$nextTick()
      await wrapper.vm.$nextTick()
      await wrapper.get('input[aria-label="Select policy type"]').trigger('focus')
      await new Promise((resolve) => window.setTimeout(resolve, 0))

      const menu = document.querySelector('.grom-select-content')
      expect(menu?.closest('dialog')).toBe(wrapper.get('dialog').element)
    } finally {
      wrapper.unmount()
    }
  })
})
