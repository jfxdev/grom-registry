import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ apiRequest: vi.fn() }))

vi.mock('@/shared/api/client', () => ({ apiRequest: mocks.apiRequest }))

import { listInventory } from './lifecycle'

describe('listInventory', () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset()
    mocks.apiRequest.mockResolvedValue({ items: [] })
  })

  it('always includes the repository and omits empty query or cursor', async () => {
    await listInventory('payments', 'api')
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(query.get('repository')).toBe('api')
    expect(query.has('q')).toBe(false)
    expect(query.has('cursor')).toBe(false)
  })

  it('includes the search query and cursor when provided', async () => {
    await listInventory('payments', 'api', 'sha256:aaaa', 'cursor-1')
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(query.get('repository')).toBe('api')
    expect(query.get('q')).toBe('sha256:aaaa')
    expect(query.get('cursor')).toBe('cursor-1')
  })
})
