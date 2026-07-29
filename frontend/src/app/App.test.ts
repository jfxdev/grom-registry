// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'

const mocks = vi.hoisted(() => ({
  getDeployment: vi.fn(),
}))

vi.mock('@/shared/api/deployment', () => ({
  getDeployment: mocks.getDeployment,
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({ user: null, signOut: vi.fn() }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ meta: { public: true }, path: '/signin' }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('App deployment warning', () => {
  beforeEach(() => {
    mocks.getDeployment.mockReset()
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
})
