import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ apiRequest: vi.fn() }))

vi.mock('@/shared/api/client', () => ({ apiRequest: mocks.apiRequest }))

import { repositorySearchKeys, searchRepositories } from './repositories'

describe('searchRepositories', () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset()
    mocks.apiRequest.mockResolvedValue({ items: [] })
  })

  it('requests the bare endpoint when no query or cursor are given', async () => {
    await searchRepositories()
    expect(mocks.apiRequest).toHaveBeenCalledWith('/api/v1/repositories')
  })

  it('includes the search query and cursor in the query string', async () => {
    await searchRepositories('api', 'cursor-1')
    const url = mocks.apiRequest.mock.calls.at(-1)![0] as string
    const query = new URLSearchParams(url.split('?')[1])
    expect(url.startsWith('/api/v1/repositories?')).toBe(true)
    expect(query.get('q')).toBe('api')
    expect(query.get('cursor')).toBe('cursor-1')
  })
})

describe('repositorySearchKeys', () => {
  it('builds a stable query key from the query and cursor', () => {
    expect(repositorySearchKeys.list('api', 'cursor-1')).toEqual(['repositories-search', 'api', 'cursor-1'])
  })
})
