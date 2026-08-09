// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserProfilePage from './UserProfilePage.vue'

const mocks = vi.hoisted(() => ({
  changePassword: vi.fn(),
  createViewerRegistryToken: vi.fn(),
  listViewerRegistryTokens: vi.fn(),
  revokeViewerRegistryToken: vi.fn(),
  user: {
    id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d',
    email: 'alex@example.com',
    username: 'alex',
    systemAdmin: false,
    systemViewer: false,
    createdAt: '2026-07-27T00:00:00Z',
  },
}))

vi.mock('@/modules/auth/api/session', () => ({
  changePassword: mocks.changePassword,
}))

vi.mock('@/modules/users/api/viewerRegistryTokens', () => ({
  createViewerRegistryToken: mocks.createViewerRegistryToken,
  listViewerRegistryTokens: mocks.listViewerRegistryTokens,
  revokeViewerRegistryToken: mocks.revokeViewerRegistryToken,
  viewerRegistryTokenKeys: { all: ['viewer-registry-tokens'] },
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({
    user: mocks.user,
  }),
}))

function mountPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return mount(UserProfilePage, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

describe('UserProfilePage', () => {
  beforeEach(() => {
	    mocks.user.systemViewer = false
    mocks.changePassword.mockReset()
    mocks.createViewerRegistryToken.mockReset()
    mocks.listViewerRegistryTokens.mockReset()
    mocks.revokeViewerRegistryToken.mockReset()
    mocks.listViewerRegistryTokens.mockResolvedValue([])
  })

  it('does not submit mismatched password confirmation', async () => {
    const wrapper = mountPage()
    const passwordInputs = wrapper.findAll('input[type="password"]')

    await passwordInputs[0]!.setValue('current-password')
    await passwordInputs[1]!.setValue('replacement-password')
    await passwordInputs[2]!.setValue('different-password')
    await wrapper.get('form').trigger('submit')

    expect(wrapper.get('[role="alert"]').text()).toContain('do not match')
    expect(mocks.changePassword).not.toHaveBeenCalled()
  })

  it('changes the password after confirming the current password', async () => {
    mocks.changePassword.mockResolvedValue(undefined)
    const wrapper = mountPage()
    const passwordInputs = wrapper.findAll('input[type="password"]')

    await passwordInputs[0]!.setValue('current-password')
    await passwordInputs[1]!.setValue('replacement-password')
    await passwordInputs[2]!.setValue('replacement-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.changePassword).toHaveBeenCalledWith(
      {
        currentPassword: 'current-password',
        newPassword: 'replacement-password',
      },
      expect.any(Object),
    )
    expect(wrapper.get('[role="status"]').text()).toContain('changed successfully')
  })

  it('lets installation viewers create and reveal a read-only registry token', async () => {
    mocks.user.systemViewer = true
    mocks.createViewerRegistryToken.mockResolvedValue({ token: { id: 'token-1', publicId: 'public', name: 'Local Docker', createdAt: '2026-08-08T00:00:00Z' }, secret: 'grm_public_secret' })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('input[placeholder="Local Docker"]').setValue('Local Docker')
    await wrapper.findAll('form').find((form) => form.text().includes('Create read-only token'))!.trigger('submit')
    await flushPromises()
    expect(mocks.createViewerRegistryToken).toHaveBeenCalledWith({ name: 'Local Docker' })
    expect(wrapper.text()).toContain('grm_public_secret')
    expect(wrapper.text()).toContain('pull only')
  })

  it('requires a viewer to revoke the active token before creating another one', async () => {
    mocks.user.systemViewer = true
    mocks.listViewerRegistryTokens.mockResolvedValue([{ id: 'token-1', publicId: 'public', name: 'Local Docker', createdAt: '2026-08-08T00:00:00Z' }])
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Revoke the active token before creating another one.')
    expect(wrapper.find('input[placeholder="Local Docker"]').exists()).toBe(false)
  })
})
