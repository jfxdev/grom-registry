// @vitest-environment jsdom

import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import AuditLogPage from './AuditLogPage.vue'

const mocks = vi.hoisted(() => ({
  listAuditEvents: vi.fn(),
}))

vi.mock('../api/auditEvents', async () => {
  const actual = await vi.importActual<typeof import('../api/auditEvents')>('../api/auditEvents')
  return {
    ...actual,
    listAuditEvents: mocks.listAuditEvents,
    auditEventKeys: {
      all: ['audit-events'],
      list: (filters: unknown, cursor = '') => ['audit-events', filters, cursor],
    },
  }
})

type Event = {
  id: string
  actorKind: string
  actorId: string
  actorName?: string
  actorUsername?: string
  action: string
  resourceKind: string
  resourceId: string
  metadata: Record<string, unknown>
  createdAt: string
}

const events: Event[] = [
  {
    id: 'e1', actorKind: 'user', actorId: 'user-1', actorName: 'alex', actorUsername: 'alex',
    action: 'projects.project_created', resourceKind: 'project', resourceId: 'proj-1',
    metadata: { reason: 'launch' }, createdAt: '2026-08-20T10:00:00Z',
  },
  {
    id: 'e2', actorKind: 'service_account', actorId: 'svc-1', actorName: 'CI Robot', actorUsername: 'ci-robot',
    action: 'identity.access_key_created', resourceKind: 'service_account', resourceId: 'svc-1',
    metadata: {}, createdAt: '2026-08-19T10:00:00Z',
  },
  {
    id: 'e3', actorKind: 'user', actorId: 'user-1', actorName: 'alex', actorUsername: 'alex',
    action: 'identity.login_succeeded', resourceKind: 'authentication', resourceId: '',
    metadata: { ip: '10.0.0.1' }, createdAt: '2026-08-18T10:00:00Z',
  },
]

function mountPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return mount(AuditLogPage, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
}

describe('AuditLogPage', () => {
  beforeAll(() => {
    HTMLElement.prototype.scrollIntoView = () => {}
  })

  beforeEach(() => {
    mocks.listAuditEvents.mockReset()
    mocks.listAuditEvents.mockImplementation((filters: { action?: string } = {}, cursor = '') => {
      let filtered = events
      if (filters.action) filtered = filtered.filter((event) => event.action === filters.action)
      const start = cursor === 'next-page' ? 2 : 0
      const items = filtered.slice(start, start + 2)
      return Promise.resolve({
        items,
        nextCursor: start + items.length < filtered.length ? 'next-page' : undefined,
      })
    })
  })

  it('shows a loading state before events resolve', () => {
    mocks.listAuditEvents.mockReturnValue(new Promise(() => {}))
    const wrapper = mountPage()
    expect(wrapper.text()).toContain('Loading events')
  })

  it('renders events with resolved actor names', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('projects.project_created')
    expect(wrapper.text()).toContain('alex')
    expect(wrapper.text()).toContain('CI Robot')
  })

  it('filters by action and resets the cursor', async () => {
    const wrapper = mountPage()
    await flushPromises()

    // Page forward first so we can prove the filter change resets the cursor.
    await wrapper.findAll('button').find((button) => button.text() === 'Next')!.trigger('click')
    await flushPromises()
    expect(mocks.listAuditEvents).toHaveBeenLastCalledWith(expect.any(Object), 'next-page')

    const actionSearch = wrapper.get('input[aria-label="Filter by action"]')
    await actionSearch.trigger('focus')
    await new Promise((resolve) => window.setTimeout(resolve, 0))
    await actionSearch.setValue('login_succeeded')
    await flushPromises()
    const loginSucceeded = Array.from(document.querySelectorAll<HTMLElement>('[role="option"]'))
      .find((option) => option.textContent?.includes('identity.login_succeeded'))
    expect(loginSucceeded).toBeDefined()
    loginSucceeded!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    const [filters, cursor] = mocks.listAuditEvents.mock.calls.at(-1)!
    expect(filters.action).toBe('identity.login_succeeded')
    expect(cursor).toBe('')
    const rows = wrapper.findAll('.audit-row')
    expect(rows).toHaveLength(1)
    expect(rows[0]!.text()).toContain('identity.login_succeeded')
    expect(rows[0]!.text()).not.toContain('projects.project_created')
  })

  it('passes RFC3339 time bounds derived from the calendar pickers', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('button[aria-label="Choose from date"]').trigger('click')
    expect(document.querySelector('[aria-label="Choose a from date"]')?.closest('.table-shell')).toBeNull()
    const fromDateButton = document.querySelector<HTMLButtonElement>('button[aria-label="Wednesday, August 19, 2026"]')
    expect(fromDateButton).not.toBeNull()
    fromDateButton!.click()
    await flushPromises()

    await wrapper.get('button[aria-label="Choose to date"]').trigger('click')
    const toDateButton = document.querySelector<HTMLButtonElement>('button[aria-label="Thursday, August 20, 2026"]')
    expect(toDateButton).not.toBeNull()
    toDateButton!.click()
    await flushPromises()

    const [filters] = mocks.listAuditEvents.mock.calls.at(-1)!
    expect(filters.from).toBe('2026-08-19T00:00:00.000Z')
    // `to` is exclusive, so the end date advances to the next day's start.
    expect(filters.to).toBe('2026-08-21T00:00:00.000Z')

    // The picker reflects the chosen dates and offers a way to clear them.
    expect(wrapper.get('button[aria-label="Choose from date"]').text()).toContain('8/19/2026')
    const fromDateGroup = wrapper.get('[aria-label="From date filter"]')
    expect(fromDateGroup.attributes('role')).toBe('group')
    const clearFromDate = wrapper.get('button[aria-label="Clear from date"]')
    expect(clearFromDate.classes()).toContain('grom-button-variant-delete')
    await clearFromDate.trigger('click')
    await flushPromises()
    expect(mocks.listAuditEvents.mock.calls.at(-1)![0].from).toBeUndefined()
  })

  it('opens a detail dialog with the full event when a row is clicked', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const row = wrapper.findAll('.audit-row').find((candidate) => candidate.text().includes('projects.project_created'))!
    await row.trigger('click')

    const dialog = wrapper.get('.audit-detail')
    expect(dialog.text()).toContain('projects.project_created')
    expect(dialog.text()).toContain('proj-1')
    expect(dialog.text()).toContain('alex')
    // Metadata is pretty-printed in the dialog.
    expect(dialog.get('.audit-metadata-value').text()).toContain('"reason": "launch"')

    await wrapper.get('button[aria-label="Close event details"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('.audit-detail').exists()).toBe(false)
  })

  it('shows an empty state when no events match', async () => {
    mocks.listAuditEvents.mockResolvedValue({ items: [] })
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('No audit events')
  })

  it('shows a retryable error instead of the empty state when loading fails', async () => {
    mocks.listAuditEvents.mockRejectedValue(new Error('request failed'))
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Unable to load audit events')
    expect(wrapper.text()).not.toContain('No audit events')

    mocks.listAuditEvents.mockResolvedValue({ items: [] })
    await wrapper.findAll('button').find((button) => button.text() === 'Retry')!.trigger('click')
    await flushPromises()
    expect(mocks.listAuditEvents).toHaveBeenCalledTimes(2)
  })
})
