// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SettingsPage from './SettingsPage.vue'

const mocks = vi.hoisted(() => ({
  getInstallationDiagnostics: vi.fn(),
  runGarbageCollection: vi.fn(),
}))

vi.mock('../api/settings', () => ({
  getInstallationDiagnostics: mocks.getInstallationDiagnostics,
  runGarbageCollection: mocks.runGarbageCollection,
}))

function diagnostics(overrides = {}) {
  return {
    checkedAt: '2026-09-07T12:00:00Z',
    deployment: { profile: 'strict', insecureHttp: false },
    database: { kind: 'postgres', status: 'available' },
    migration: { status: 'current', appliedVersion: '202608300001', appliedAt: '2026-09-07T11:00:00Z' },
    signing: { status: 'loaded', algorithm: 'RS256', keyId: 'grom-default' },
    distribution: { status: 'available', apiVersion: 'registry/2.0' },
    storage: { status: 'available', usedBytes: 2048 },
    backup: { status: 'available', lastBackupAt: '2026-09-07T10:00:00Z' },
    ...overrides,
  }
}

function mountPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return mount(SettingsPage, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
}

describe('SettingsPage', () => {
  beforeEach(() => {
    mocks.getInstallationDiagnostics.mockReset()
    mocks.runGarbageCollection.mockReset()
    mocks.getInstallationDiagnostics.mockResolvedValue(diagnostics())
    mocks.runGarbageCollection.mockResolvedValue({ reclaimedBytes: 1024 })
  })

  it('renders live diagnostics and refreshes them manually', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Installation')
    expect(wrapper.text()).toContain('Diagnostics')
    expect(wrapper.text()).toContain('PostgreSQL')
    expect(wrapper.text()).toContain('202608300001')
    expect(wrapper.text()).toContain('registry/2.0')
    expect(wrapper.text()).toContain('RS256')
    expect(wrapper.text()).toContain('grom-default')
    expect(wrapper.text()).toContain('Last backup')
    expect(wrapper.text()).toContain('Maintenance')
    expect(wrapper.text()).toContain('Run garbage collection')

    await wrapper.get('[aria-label="Refresh diagnostics"]').trigger('click')
    await flushPromises()
    expect(mocks.getInstallationDiagnostics).toHaveBeenCalledTimes(2)
  })

  it('keeps partial diagnostics useful and disables maintenance when storage is unavailable', async () => {
    mocks.getInstallationDiagnostics.mockResolvedValue(diagnostics({
      deployment: { profile: 'permissive', insecureHttp: true },
      migration: { status: 'pending', appliedVersion: null, appliedAt: null },
      signing: { status: 'unavailable', algorithm: null, keyId: null },
      distribution: { status: 'available', apiVersion: null },
      storage: { status: 'unavailable', usedBytes: null },
      backup: { status: 'available', lastBackupAt: null },
    }))
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Pending')
    expect(wrapper.text()).toContain('Not reported')
    expect(wrapper.text()).toContain('No recovery point yet')
    expect(wrapper.text()).toContain('Insecure HTTP')
    const garbageCollection = wrapper.findAll('button').find((button) => button.text().includes('Run garbage collection'))
    expect(garbageCollection?.attributes('disabled')).toBeDefined()
  })

  it('keeps garbage collection in the maintenance section and refreshes diagnostics after completion', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const garbageCollection = wrapper.findAll('button').find((button) => button.text().includes('Run garbage collection'))
    await garbageCollection!.trigger('click')
    await flushPromises()

    expect(mocks.runGarbageCollection).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Reclaimed')
    expect(mocks.getInstallationDiagnostics).toHaveBeenCalledTimes(2)
  })
})
