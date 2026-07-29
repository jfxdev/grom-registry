// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ResetPasswordPage from './ResetPasswordPage.vue'

const mocks = vi.hoisted(() => ({
  completePasswordReset: vi.fn(),
}))

vi.mock('@/modules/auth/api/session', () => ({
  completePasswordReset: mocks.completePasswordReset,
}))

function mountPage(token = 'grmpr_public_secret') {
  window.history.replaceState(null, '', `/reset-password#token=${token}`)
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return mount(ResetPasswordPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
}

describe('ResetPasswordPage', () => {
  beforeEach(() => {
    mocks.completePasswordReset.mockReset()
  })

  it('rejects mismatched confirmation before submitting', async () => {
    const wrapper = mountPage()
    const inputs = wrapper.findAll('input[type="password"]')

    await inputs[0]!.setValue('replacement-password')
    await inputs[1]!.setValue('different-password')
    await wrapper.get('form').trigger('submit')

    expect(wrapper.get('[role="alert"]').text()).toContain('do not match')
    expect(mocks.completePasswordReset).not.toHaveBeenCalled()
  })

  it('submits the fragment token and shows completion', async () => {
    mocks.completePasswordReset.mockResolvedValue(undefined)
    const wrapper = mountPage('grmpr_public_once')
    const inputs = wrapper.findAll('input[type="password"]')

    await inputs[0]!.setValue('replacement-password')
    await inputs[1]!.setValue('replacement-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.completePasswordReset).toHaveBeenCalledWith(
      { token: 'grmpr_public_once', newPassword: 'replacement-password' },
      expect.any(Object),
    )
    expect(wrapper.text()).toContain('Password reset')
    expect(window.location.hash).toBe('')
  })
})
