// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'

const mocks = vi.hoisted(() => ({
  getDeployment: vi.fn(),
  route: { meta: { public: true }, path: '/signin' },
  router: { push: vi.fn() },
  session: { user: null as { username: string, email: string, systemAdmin: boolean } | null, signOut: vi.fn() },
}))

vi.mock('@/shared/api/deployment', () => ({
  getDeployment: mocks.getDeployment,
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => mocks.session,
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => mocks.router,
}))

describe('App deployment warning', () => {
  beforeEach(() => {
    mocks.getDeployment.mockReset()
    mocks.router.push.mockReset()
    mocks.session.signOut.mockReset()
    mocks.session.user = null
    mocks.route.meta.public = true
    mocks.route.path = '/signin'
  })

  it('clearly identifies explicitly permitted insecure HTTP', async () => {
    mocks.getDeployment.mockResolvedValue({ profile: 'permissive', insecureHttp: true })

    const wrapper = mount(App, {
      global: {
        stubs: {
          RouterView: { template: '<main>Public page</main>' },
          RouterLink: { template: '<a><slot /></a>' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('Insecure HTTP deployment')
    expect(wrapper.text()).toContain('trusted private network')
  })

  it('does not show a warning for a strict deployment', async () => {
    mocks.getDeployment.mockResolvedValue({ profile: 'strict', insecureHttp: false })

    const wrapper = mount(App, {
      global: {
        stubs: {
          RouterView: { template: '<main>Public page</main>' },
          RouterLink: { template: '<a><slot /></a>' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('opens the account menu in the top bar and signs out from it', async () => {
    mocks.getDeployment.mockResolvedValue({ profile: 'strict', insecureHttp: false })
    mocks.route.meta.public = false
    mocks.route.path = '/projects'
    mocks.session.user = { username: 'Avery', email: 'avery@example.test', systemAdmin: false }

    const wrapper = mount(App, {
      global: {
        stubs: {
          RouterView: { template: '<main>Projects</main>' },
          RouterLink: { template: '<a><slot /></a>' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.sidebar-footer').exists()).toBe(false)
    expect(wrapper.find('#desktop-account-menu').exists()).toBe(false)

    await wrapper.get('[aria-controls="desktop-account-menu"]').trigger('click')

    const accountMenu = wrapper.get('#desktop-account-menu')
    expect(accountMenu.text()).toContain('Profile')
    expect(accountMenu.text()).toContain('Sign out')

    await accountMenu.get('button').trigger('click')
    await flushPromises()

    expect(mocks.session.signOut).toHaveBeenCalledOnce()
    expect(mocks.router.push).toHaveBeenCalledWith('/signin')
  })
})
