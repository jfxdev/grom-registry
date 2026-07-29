// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BackupsPage from './BackupsPage.vue'

const mocks = vi.hoisted(() => ({
  createBackup: vi.fn(),
  deleteBackup: vi.fn(),
  getBackupOverview: vi.fn(),
}))

vi.mock('../api/backups', () => ({
  backupKeys: {
    all: ['backups'],
    overview: (cursor = '') => ['backups', 'overview', cursor],
  },
  backupDownloadURL: (id: string) => `/api/v1/backups/${id}/download`,
  createBackup: mocks.createBackup,
  deleteBackup: mocks.deleteBackup,
  getBackupOverview: mocks.getBackupOverview,
}))

function mountPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return mount(BackupsPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

describe('BackupsPage', () => {
  beforeEach(() => {
    mocks.createBackup.mockReset()
    mocks.deleteBackup.mockReset()
    mocks.getBackupOverview.mockReset()
    mocks.getBackupOverview.mockResolvedValue({
      available: true,
      activeOperation: null,
      totalBackups: 1,
      pageSize: 5,
      nextCursor: null,
      backups: [{
        backupId: '9bb21d94-87a7-4c6c-a677-1832d7ba56c7',
        createdAt: '2026-07-29T16:00:00Z',
        gromVersion: '0.1.0',
        totalBytes: 2048,
      }],
    })
    mocks.createBackup.mockResolvedValue({
      id: '49c3d4b2-b4a2-4c09-aa31-790885ad4bdb',
      status: 'starting',
      startedAt: '2026-07-29T16:01:00Z',
    })
    mocks.deleteBackup.mockResolvedValue(undefined)
  })

  it('confirms backup creation and shows downloadable recovery points', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('2.00 KiB')
    expect(wrapper.text()).toContain('0.1.0')
    expect(wrapper.text()).toContain('View details')
    expect(wrapper.text()).not.toContain('git clone https://github.com/jfxdev/grom-registry.git')

    const details = wrapper.findAll('button').find(button => button.text().includes('View details'))
    expect(details).toBeDefined()
    await details!.trigger('click')
    expect(wrapper.text()).toContain('Download a recovery bundle before an incident')
    expect(wrapper.text()).toContain('encrypted off-host storage')
    expect(wrapper.text()).toContain('docker compose')
    expect(wrapper.text()).toContain('git clone https://github.com/jfxdev/grom-registry.git')
    expect(wrapper.text()).toContain('http://127.0.0.1:8081')
    expect(wrapper.text()).toContain('Never delete a healthy installation')

    const close = wrapper.findAll('button').find(button => button.text().includes('Close'))
    expect(close).toBeDefined()
    await close!.trigger('click')
    expect(wrapper.text()).not.toContain('git clone https://github.com/jfxdev/grom-registry.git')

    await wrapper.get('header button').trigger('click')
    expect(wrapper.text()).toContain('Create recovery point?')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.createBackup).toHaveBeenCalledOnce()
  })

  it('requires typed confirmation before deleting a recovery point', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const deleteButton = wrapper.get('[aria-label="Delete backup 9bb21d94-87a7-4c6c-a677-1832d7ba56c7"]')
    expect(deleteButton.classes()).toContain('grom-button-size-icon')
    expect(deleteButton.classes()).toContain('grom-button-variant-delete')
    expect(deleteButton.text()).toBe('')
    await deleteButton.trigger('click')
    expect(wrapper.text()).toContain('Delete this recovery point?')
    const confirmationButton = wrapper.get('button[type="submit"]')
    expect(confirmationButton.classes()).toContain('grom-button-variant-delete')
    expect(confirmationButton.text()).toContain('Delete snapshot')
    expect(confirmationButton.attributes('disabled')).toBeDefined()

    await wrapper.get('[aria-label="Type DELETE to confirm"]').setValue('DELETE')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mocks.deleteBackup.mock.calls[0]?.[0]).toBe('9bb21d94-87a7-4c6c-a677-1832d7ba56c7')
  })

  it('loads the next five-item page with the opaque cursor', async () => {
    mocks.getBackupOverview.mockImplementation((cursor = '') => Promise.resolve(cursor ? {
      available: true,
      activeOperation: null,
      totalBackups: 6,
      pageSize: 5,
      nextCursor: null,
      backups: [{
        backupId: '1bb21d94-87a7-4c6c-a677-1832d7ba56c7',
        createdAt: '2026-07-29T15:00:00Z',
        gromVersion: '0.1.0',
        totalBytes: 1024,
      }],
    } : {
      available: true,
      activeOperation: null,
      totalBackups: 6,
      pageSize: 5,
      nextCursor: 'next-page',
      backups: [{
        backupId: '9bb21d94-87a7-4c6c-a677-1832d7ba56c7',
        createdAt: '2026-07-29T16:00:00Z',
        gromVersion: '0.1.0',
        totalBytes: 2048,
      }],
    }))
    const wrapper = mountPage()
    await flushPromises()

    const next = wrapper.findAll('button').find(button => button.text().includes('Next'))
    expect(next).toBeDefined()
    await next!.trigger('click')
    await flushPromises()

    expect(mocks.getBackupOverview).toHaveBeenCalledWith('next-page')
    expect(wrapper.text()).toContain('Page 2')
  })

  it('disables creation when the internal agent is unavailable', async () => {
    mocks.getBackupOverview.mockResolvedValue({
      available: false, activeOperation: null, backups: [], totalBackups: 0, pageSize: 5, nextCursor: null,
    })
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Integrated backup agent unavailable')
    expect(wrapper.get('header button').attributes('disabled')).toBeDefined()
  })
})
