// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SignInPage from './SignInPage.vue'

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  signIn: vi.fn(),
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({ signIn: mocks.signIn }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: mocks.push }),
}))

describe('SignInPage', () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.signIn.mockReset()
  })

  it('toggles password visibility with an accessible control', async () => {
    const wrapper = mount(SignInPage)
    const password = wrapper.get('input[autocomplete="current-password"]')
    const toggle = wrapper.get('button[aria-label="Show password"]')

    expect(password.attributes('type')).toBe('password')
    expect(password.element.closest('label')?.contains(toggle.element)).toBe(false)
    await toggle.trigger('click')
    expect(password.attributes('type')).toBe('text')
    expect(wrapper.get('button[aria-label="Hide password"]').attributes('aria-label')).toBe('Hide password')
  })

  it('explains that new users require an invitation', () => {
    const wrapper = mount(SignInPage)

    expect(wrapper.text()).toContain('New users are invited by an administrator.')
  })

  it('renders a sign-in error as an alert', async () => {
    mocks.signIn.mockRejectedValue(new Error('no session'))
    const wrapper = mount(SignInPage)

    await wrapper.get('input[autocomplete="current-password"]').setValue('wrong-password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toBe('Sign in failed')
    expect(mocks.push).not.toHaveBeenCalled()
  })

  it('redirects to projects after a successful sign-in', async () => {
    mocks.signIn.mockResolvedValue(undefined)
    const wrapper = mount(SignInPage)

    await wrapper.get('input[autocomplete="current-password"]').setValue('password')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.signIn).toHaveBeenCalledWith('admin@grom.local', 'password')
    expect(mocks.push).toHaveBeenCalledWith('/projects')
  })
})
