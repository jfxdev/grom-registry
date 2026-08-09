// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ServiceAccountsPage from './ServiceAccountsPage.vue'

const mocks = vi.hoisted(() => ({
  createServiceAccount: vi.fn(),
  disableServiceAccount: vi.fn(),
  listServiceAccounts: vi.fn(),
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({ user: { systemAdmin: true } }),
}))

vi.mock('../components/ServiceAccountKeysPanel.vue', () => ({
  default: { template: '<div />' },
}))

vi.mock('../api/serviceAccounts', () => ({
  createServiceAccount: mocks.createServiceAccount,
  disableServiceAccount: mocks.disableServiceAccount,
  listServiceAccounts: mocks.listServiceAccounts,
  serviceAccountKeys: {
    all: ['service-accounts'],
    list: (includeDisabled = false) => ['service-accounts', includeDisabled ? 'all' : 'active'],
    tokens: (id: string) => ['service-accounts', id, 'tokens'],
  },
}))

function mountPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return mount(ServiceAccountsPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { ServiceAccountKeysPanel: true },
    },
  })
}

describe('ServiceAccountsPage', () => {
  beforeEach(() => {
    mocks.createServiceAccount.mockReset()
    mocks.disableServiceAccount.mockReset()
    mocks.disableServiceAccount.mockResolvedValue(undefined)
    mocks.listServiceAccounts.mockReset()
    mocks.listServiceAccounts.mockResolvedValue([
      {
        id: 'account-1',
        name: 'Payments CI',
        username: 'ci-payments',
        description: 'Production deployment pipeline',
        createdAt: '2026-07-29T10:00:00Z',
      },
      {
        id: 'account-2',
        name: 'Registry mirror',
        username: 'mirror-pull',
        description: 'Synchronizes base images',
        createdAt: '2026-07-29T10:00:00Z',
        disabledAt: '2026-07-29T12:00:00Z',
      },
    ])
  })

  it('shows a loading state before service accounts resolve', () => {
    mocks.listServiceAccounts.mockReturnValue(new Promise(() => {}))
    const wrapper = mountPage()

    expect(wrapper.text()).toContain('Loading service accounts')
    expect(wrapper.text()).not.toContain('No active service accounts')
  })

  it('filters service accounts by name, username or description', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const disableButton = wrapper.get('button[aria-label="Disable service account Payments CI"]')
    expect(disableButton.classes()).toContain('grom-button-size-icon')
    expect(disableButton.classes()).toContain('grom-button-variant-delete')
    expect(disableButton.text()).toBe('')

    const search = wrapper.get('input[aria-label="Search service accounts"]')
    await search.setValue('CI-PAYMENTS')
    expect(wrapper.text()).toContain('Payments CI')
    expect(wrapper.text()).not.toContain('Registry mirror')
    expect(wrapper.text()).toContain('1 of 1 on this page')

    await wrapper.get('select[aria-label="Filter service accounts by status"]').setValue('disabled')
    await search.setValue('base images')
    expect(wrapper.text()).toContain('Registry mirror')
    expect(wrapper.text()).not.toContain('Payments CI')
    expect(wrapper.text()).toContain('Disabled')
  })

  it('shows an empty search state when no service account matches', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('input[aria-label="Search service accounts"]').setValue('missing')

    expect(wrapper.text()).toContain('No matching service accounts')
    expect(wrapper.text()).toContain('0 of 1 on this page')
  })

  it('requires the exact service account name before disabling it', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Disable service account Payments CI"]').trigger('click')
    expect(wrapper.text()).toContain('Disable service account')
    expect(wrapper.text()).toContain('Type Payments CI to confirm')
    expect(wrapper.text()).toContain('Automations using those keys will lose access to the registry')

    const confirmationButton = wrapper.get('button[type="submit"]')
    expect(confirmationButton.classes()).toContain('grom-button-variant-delete')
    expect(confirmationButton.attributes('disabled')).toBeDefined()

    const confirmation = wrapper.get('input[aria-label="Service account name confirmation"]')
    await confirmation.setValue('payments ci')
    await wrapper.get('form[aria-labelledby="disable-service-account-title"]').trigger('submit')
    expect(mocks.disableServiceAccount).not.toHaveBeenCalled()

    await confirmation.setValue('Payments CI')
    await wrapper.get('form[aria-labelledby="disable-service-account-title"]').trigger('submit')
    await flushPromises()

    expect(mocks.disableServiceAccount).toHaveBeenCalledOnce()
    expect(mocks.disableServiceAccount.mock.calls[0]?.[0]).toBe('account-1')
  })
})
