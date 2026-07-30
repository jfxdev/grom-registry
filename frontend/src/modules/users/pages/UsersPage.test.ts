// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UsersPage from './UsersPage.vue'

const mocks = vi.hoisted(() => ({
  createUser: vi.fn(),
  createUserPasswordResetLink: vi.fn(),
  listUsers: vi.fn(),
}))

vi.mock('../api/users', () => ({
  createUser: mocks.createUser,
  createUserPasswordResetLink: mocks.createUserPasswordResetLink,
  listUsers: mocks.listUsers,
  userKeys: { all: ['users'] },
}))

function mountPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return mount(UsersPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

describe('UsersPage', () => {
  beforeEach(() => {
    mocks.createUser.mockReset()
    mocks.createUserPasswordResetLink.mockReset()
    mocks.listUsers.mockReset()
    mocks.listUsers.mockResolvedValue([
      {
        id: 'user-1',
        email: 'alex@example.com',
        username: 'alex',
        systemAdmin: true,
        createdAt: '2026-07-29T10:00:00Z',
      },
      {
        id: 'user-2',
        email: 'sam@registry.test',
        username: 'sam',
        systemAdmin: false,
        createdAt: '2026-07-29T10:00:00Z',
      },
    ])
  })

  it('shows a loading state before users resolve', () => {
    mocks.listUsers.mockReturnValue(new Promise(() => {}))
    const wrapper = mountPage()

    expect(wrapper.text()).toContain('Loading users')
    expect(wrapper.text()).not.toContain('No users yet')
  })

  it('filters users by username or email', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const search = wrapper.get('input[aria-label="Search users"]')
    await search.setValue('ALEX')
    expect(wrapper.text()).toContain('alex@example.com')
    expect(wrapper.text()).not.toContain('sam@registry.test')
    expect(wrapper.text()).toContain('1 of 2')

    await search.setValue('registry.test')
    expect(wrapper.text()).toContain('sam@registry.test')
    expect(wrapper.text()).not.toContain('alex@example.com')
  })

  it('shows an empty search state when no user matches', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('input[aria-label="Search users"]').setValue('missing')

    expect(wrapper.text()).toContain('No matching users')
    expect(wrapper.text()).toContain('0 of 2')
  })
})
