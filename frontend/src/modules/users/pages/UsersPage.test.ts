// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/shared/api/client'
import UsersPage from './UsersPage.vue'

const mocks = vi.hoisted(() => ({
  createUser: vi.fn(),
  createUserPasswordResetLink: vi.fn(),
  disableUser: vi.fn(),
  listUsers: vi.fn(),
  promoteUserToSystemAdmin: vi.fn(),
  promoteUserToSystemViewer: vi.fn(),
}))

vi.mock('../api/users', () => ({
  createUser: mocks.createUser,
  createUserPasswordResetLink: mocks.createUserPasswordResetLink,
  disableUser: mocks.disableUser,
  listUsers: mocks.listUsers,
  promoteUserToSystemAdmin: mocks.promoteUserToSystemAdmin,
  promoteUserToSystemViewer: mocks.promoteUserToSystemViewer,
  userKeys: { all: ['users'] },
}))

vi.mock('@/modules/auth/store/session', () => ({
  useSessionStore: () => ({ user: { id: 'admin-1', systemAdmin: true } }),
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
    mocks.disableUser.mockReset()
    mocks.listUsers.mockReset()
    mocks.promoteUserToSystemAdmin.mockReset()
    mocks.promoteUserToSystemViewer.mockReset()
    mocks.listUsers.mockResolvedValue({
      items: [
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
      ],
      pageCount: 1,
    })
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
    expect(wrapper.text()).toContain('1 of 2 on this page')

    await search.setValue('registry.test')
    expect(wrapper.text()).toContain('sam@registry.test')
    expect(wrapper.text()).not.toContain('alex@example.com')
  })

  it('shows an empty search state when no user matches', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('input[aria-label="Search users"]').setValue('missing')

    expect(wrapper.text()).toContain('No matching users')
    expect(wrapper.text()).toContain('0 of 2 on this page')
  })

  it('labels the loaded users as the current page rather than a global total', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('2 on this page')
    expect(wrapper.text()).not.toContain('2 total')
  })

  it('confirms and disables a user', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Disable user sam"]').trigger('click')
    expect(wrapper.text()).toContain('This revokes the user’s active sessions')
    mocks.disableUser.mockResolvedValue(undefined)
    await wrapper.get('form[aria-labelledby="disable-user-title"]').trigger('submit')
    expect(mocks.disableUser).toHaveBeenCalledWith('user-2')
  })

  it('creates regular users and changes roles through a role dropdown', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('New user'))!.trigger('click')
    await wrapper.get('input[type="email"]').setValue('new@example.com')
    await wrapper.get('input[placeholder="alex"]').setValue('new-user')
    mocks.createUser.mockResolvedValueOnce({
      user: { id: 'user-3', email: 'new@example.com', username: 'new-user', systemAdmin: false, createdAt: '2026-08-08T00:00:00Z' },
      registrationLink: { url: 'https://grom.example/reset-password#token=registration-token', expiresAt: '2026-08-08T00:30:00Z' },
    })
    await wrapper.findAll('form').find((form) => form.text().includes('Create user'))!.trigger('submit')
    expect(mocks.createUser.mock.calls[0]![0]).toEqual({ email: 'new@example.com', username: 'new-user' })
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('Copy this registration link now')
    expect(wrapper.text()).toContain('registration-token')

    const roleButton = wrapper.get('button[aria-label="Change role for sam"]')
    expect(wrapper.text()).toContain('Administrator')
    expect(wrapper.text()).toContain('User')
    await roleButton.trigger('click')
    expect(wrapper.get('[role="menu"]').text()).toContain('Viewer')
    await wrapper.findAll('[role="menuitem"]').find((item) => item.text().includes('Viewer'))!.trigger('click')
    mocks.promoteUserToSystemViewer.mockResolvedValueOnce(undefined)
    await wrapper.get('form[aria-labelledby="promote-viewer-title"]').trigger('submit')
    expect(mocks.promoteUserToSystemViewer).toHaveBeenCalledWith('user-2')

    await roleButton.trigger('click')
    await wrapper.findAll('[role="menuitem"]').find((item) => item.text().includes('Administrator'))!.trigger('click')
    expect(wrapper.text()).toContain('Make sam an administrator?')
    mocks.promoteUserToSystemAdmin.mockResolvedValueOnce(undefined)
    await wrapper.get('form[aria-labelledby="promote-user-title"]').trigger('submit')
    expect(mocks.promoteUserToSystemAdmin.mock.calls[0]![0]).toBe('user-2')
  })

  it('offers only promotion to administrator for viewers', async () => {
    mocks.listUsers.mockResolvedValueOnce({
      items: [{
        id: 'viewer-1', email: 'viewer@example.com', username: 'viewer', systemAdmin: false, systemViewer: true,
        createdAt: '2026-08-08T00:00:00Z',
      }],
      pageCount: 1,
    })
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Change role for viewer"]').trigger('click')
    const roleMenu = wrapper.get('[role="menu"]')
    expect(roleMenu.text()).not.toContain('Viewer')
    expect(roleMenu.text()).toContain('Administrator')
    await roleMenu.find('[role="menuitem"]').trigger('click')
    mocks.promoteUserToSystemAdmin.mockResolvedValueOnce(undefined)
    await wrapper.get('form[aria-labelledby="promote-user-title"]').trigger('submit')
    expect(mocks.promoteUserToSystemAdmin).toHaveBeenCalledWith('viewer-1')
  })

  it.each([
    ['username_taken', 'This username is already in use.'],
    ['email_taken', 'This email address is already in use.'],
  ])('shows a clear message for duplicate user %s', async (errorCode, expectedMessage) => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('New user'))!.trigger('click')
    mocks.createUser.mockRejectedValueOnce(new APIError(409, errorCode, 'Could not create user'))
    await wrapper.findAll('form').find((form) => form.text().includes('Create user'))!.trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain(expectedMessage)
    expect(wrapper.text()).not.toContain('Could not create user')
  })

  it('does not offer password reset or disable actions for disabled users', async () => {
    mocks.listUsers.mockResolvedValueOnce({
      items: [{
        id: 'user-3', email: 'disabled@example.com', username: 'disabled', systemAdmin: false,
        createdAt: '2026-07-29T10:00:00Z', disabledAt: '2026-07-30T10:00:00Z',
      }],
      pageCount: 1,
    })
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[aria-label="Inactive user"]')).toBeDefined()
    expect(wrapper.text()).not.toContain('Reset password')
    expect(wrapper.find('button[aria-label="Disable user disabled"]').exists()).toBe(false)
  })

  it('disables the action for the signed-in administrator account', async () => {
    mocks.listUsers.mockResolvedValueOnce({
      items: [{
        id: 'admin-1', email: 'admin@example.com', username: 'admin', systemAdmin: true,
        createdAt: '2026-07-29T10:00:00Z',
      }],
      pageCount: 1,
    })
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('button[aria-label="Disable user admin"]').attributes('disabled')).toBeDefined()
  })

  it('shows disable errors from the API', async () => {
    mocks.disableUser.mockRejectedValueOnce(new Error('service unavailable'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Disable user sam"]').trigger('click')
    await wrapper.get('form[aria-labelledby="disable-user-title"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('Could not disable the user')
  })
})
