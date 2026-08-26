import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ apiRequest: vi.fn() }))

vi.mock('@/shared/api/client', () => ({ apiRequest: mocks.apiRequest }))

import { listAuditEvents, auditEventKeys, type AuditFilters } from './auditEvents'

describe('listAuditEvents', () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset()
    mocks.apiRequest.mockResolvedValue({ items: [] })
  })

  it('requests the bare endpoint when no filters are given', async () => {
    await listAuditEvents()
    expect(mocks.apiRequest).toHaveBeenCalledWith('/api/v1/audit-events')
  })

  it('serializes every filter and the cursor into the query string', async () => {
    await listAuditEvents(
      {
        action: 'identity.login_succeeded',
        resource: 'user',
        actor: 'alex',
        from: '2026-08-19T00:00:00.000Z',
        to: '2026-08-21T00:00:00.000Z',
      },
      'cursor-1',
    )
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(query.get('action')).toBe('identity.login_succeeded')
    expect(query.get('resource')).toBe('user')
    expect(query.get('actor')).toBe('alex')
    expect(query.get('from')).toBe('2026-08-19T00:00:00.000Z')
    expect(query.get('to')).toBe('2026-08-21T00:00:00.000Z')
    expect(query.get('cursor')).toBe('cursor-1')
  })

  it('omits empty filters', async () => {
    await listAuditEvents({ action: 'projects.project_created' })
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(query.get('action')).toBe('projects.project_created')
    expect(query.has('resource')).toBe(false)
    expect(query.has('cursor')).toBe(false)
  })

  it('builds a stable query key from filters and cursor', () => {
    const filters: AuditFilters = { action: 'identity.login_succeeded' }
    expect(auditEventKeys.list(filters, 'c1')).toEqual(['audit-events', filters, 'c1'])
    expect(auditEventKeys.all).toEqual(['audit-events'])
  })
})
