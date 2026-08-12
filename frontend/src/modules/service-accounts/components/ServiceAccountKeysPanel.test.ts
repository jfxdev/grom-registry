// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ServiceAccountKeysPanel from './ServiceAccountKeysPanel.vue'

const mocks = vi.hoisted(() => ({
  listServiceAccountTokens: vi.fn(),
}))

vi.mock('../api/serviceAccounts', () => ({
  createServiceAccountToken: vi.fn(),
  listServiceAccountTokens: mocks.listServiceAccountTokens,
  revokeServiceAccountToken: vi.fn(),
  serviceAccountKeys: { tokens: (id: string) => ['service-accounts', id, 'tokens'] },
}))

function mountPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return mount(ServiceAccountKeysPanel, {
    props: {
      account: { id: 'account-1', name: 'Payments CI', username: 'payments-ci', description: '', createdAt: '2026-08-11T00:00:00Z' },
    },
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

describe('ServiceAccountKeysPanel', () => {
  beforeEach(() => {
    mocks.listServiceAccountTokens.mockReset()
    mocks.listServiceAccountTokens.mockResolvedValue({
      items: [
        { id: 'key-1', publicId: 'key-1', name: 'CI', serviceAccountId: 'account-1', createdAt: '2026-08-11T00:00:00Z' },
        { id: 'key-2', publicId: 'key-2', name: 'Deploy', serviceAccountId: 'account-1', createdAt: '2026-08-11T00:00:00Z' },
        { id: 'key-3', publicId: 'key-3', name: 'Release', serviceAccountId: 'account-1', createdAt: '2026-08-11T00:00:00Z' },
      ],
    })
  })

  it('prevents creating a fourth active key and does not render pagination', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.findAll('button').find((button) => button.text().includes('New key'))?.attributes('disabled')).toBeDefined()
    expect(wrapper.find('[aria-label="Pagination"]').exists()).toBe(false)
  })

  it('creates a key from a dialog with the calendar expiration picker', async () => {
    mocks.listServiceAccountTokens.mockResolvedValueOnce({ items: [] })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('New key'))!.trigger('click')

    expect(wrapper.get('dialog[aria-labelledby="create-access-key-title"]').text()).toContain('Create access key')
    expect(wrapper.find('dialog [aria-label="Previous month"]').exists()).toBe(true)
    expect(wrapper.find('dialog input[type="datetime-local"]').exists()).toBe(false)
  })
})
