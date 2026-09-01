import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ apiRequest: vi.fn() }))

vi.mock('@/shared/api/client', () => ({ apiRequest: mocks.apiRequest }))

import { listRepositories, listTags } from './projects'

describe('listRepositories', () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset()
    mocks.apiRequest.mockResolvedValue({ items: [] })
  })

  it('requests the bare endpoint when no query or cursor are given', async () => {
    await listRepositories('payments')
    expect(mocks.apiRequest).toHaveBeenCalledWith('/api/v1/projects/payments/repositories')
  })

  it('includes the search query and cursor in the query string', async () => {
    await listRepositories('payments', 'api', 'cursor-1')
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(url.startsWith('/api/v1/projects/payments/repositories?')).toBe(true)
    expect(query.get('q')).toBe('api')
    expect(query.get('cursor')).toBe('cursor-1')
  })
})

describe('listTags', () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset()
    mocks.apiRequest.mockResolvedValue({ items: [] })
  })

  it('always includes the repository and omits empty query or cursor', async () => {
    await listTags('payments', 'api')
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(query.get('repository')).toBe('api')
    expect(query.has('q')).toBe(false)
    expect(query.has('cursor')).toBe(false)
  })

  it('includes the search query and cursor when provided', async () => {
    await listTags('payments', 'api', 'v1', 'cursor-1')
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(query.get('repository')).toBe('api')
    expect(query.get('q')).toBe('v1')
    expect(query.get('cursor')).toBe('cursor-1')
  })
})
