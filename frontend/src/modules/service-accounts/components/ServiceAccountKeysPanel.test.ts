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

function mountPanel(attachTo?: HTMLElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return mount(ServiceAccountKeysPanel, {
    attachTo,
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
      activeCount: 3,
      maxActiveCount: 3,
      nextCursor: 'next-page',
    })
  })

  it('uses the server-wide active count and renders token pagination', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.findAll('button').find((button) => button.text().includes('New key'))?.attributes('disabled')).toBeDefined()
    expect(wrapper.find('[aria-label="Pagination"]').exists()).toBe(true)

    await wrapper.findAll('button').find((button) => button.text() === 'Next')!.trigger('click')
    await flushPromises()

    expect(mocks.listServiceAccountTokens).toHaveBeenCalledWith('account-1', 'next-page')
  })

  it('creates a key from a dialog with the calendar expiration picker', async () => {
    mocks.listServiceAccountTokens.mockResolvedValueOnce({ items: [], activeCount: 0, maxActiveCount: 3 })
    const wrapper = mountPanel(document.body)
    await flushPromises()

    const trigger = wrapper.findAll('button').find((button) => button.text().includes('New key'))!
    trigger.element.focus()
    await trigger.trigger('click')

    expect(wrapper.get('dialog[aria-labelledby="create-access-key-title"]').text()).toContain('Create access key')
    expect(wrapper.find('dialog [aria-label="Previous month"]').exists()).toBe(true)
    expect(wrapper.find('dialog input[type="datetime-local"]').exists()).toBe(false)

    await wrapper.get('dialog[aria-labelledby="create-access-key-title"]').trigger('cancel')
    await flushPromises()
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })
})
