// @vitest-environment jsdom

import type { Repository } from '@/shared/api/models'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text().includes('Add policy'))!.trigger('click')
    expect(wrapper.text()).toContain('Expire after days')

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
            keepLast: 10,
            untaggedGraceDays: 7,
          }),
        ],
      }),
    )
    expect(wrapper.emitted('saved')?.[0]?.[0]).toMatchObject({ version: 1 })
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
})
