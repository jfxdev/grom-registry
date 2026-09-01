// @vitest-environment jsdom

import { APIError } from '@/shared/api/client'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import RepositorySearchPage from './RepositorySearchPage.vue'

const mocks = vi.hoisted(() => ({
  searchRepositories: vi.fn(),
  routerPush: vi.fn(),
}))

vi.mock('../api/repositories', () => ({
  searchRepositories: mocks.searchRepositories,
  repositorySearchKeys: { list: (query: string, cursor = '') => ['repositories-search', query, cursor] },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.routerPush }),
}))

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(RepositorySearchPage, {
    global: { plugins: [[VueQueryPlugin, { queryClient }]] },
  })
}

describe('RepositorySearchPage', () => {
  beforeEach(() => {
    mocks.searchRepositories.mockReset()
    mocks.routerPush.mockReset()
    mocks.searchRepositories.mockResolvedValue({ items: [] })
  })

  afterEach(() => {
    cleanup()
  })

  it('shows a prompt to type before any search has run', async () => {
    renderPage()
    await flushPromises()

    expect(await screen.findByText('Type to search')).toBeTruthy()
    expect(mocks.searchRepositories).not.toHaveBeenCalled()
  })

  it('searches on the server and lists matches with their project', async () => {
    mocks.searchRepositories.mockImplementation((query: string) => Promise.resolve(query === 'api' ? {
      items: [{ id: 'repository-1', projectId: 'project-1', projectSlug: 'payments', projectName: 'Payments', name: 'api', description: '', status: 'active', createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z' }],
    } : { items: [] }))
    renderPage()

    await fireEvent.update(await screen.findByRole('searchbox', { name: 'Search repositories' }), 'api')

    await waitFor(() => expect(mocks.searchRepositories).toHaveBeenLastCalledWith('api', ''))
    expect(await screen.findByText('payments/api')).toBeTruthy()
    expect(screen.getByText('Payments')).toBeTruthy()
  })

  it('shows a no-matches empty state', async () => {
    mocks.searchRepositories.mockResolvedValue({ items: [] })
    renderPage()

    await fireEvent.update(await screen.findByRole('searchbox', { name: 'Search repositories' }), 'nothing-like-this')

    expect(await screen.findByText('No matching repositories')).toBeTruthy()
  })

  it('navigates to the repository detail page when a result is selected', async () => {
    mocks.searchRepositories.mockResolvedValue({
      items: [{ id: 'repository-1', projectId: 'project-1', projectSlug: 'payments', projectName: 'Payments', name: 'api', description: '', status: 'active', createdAt: '2026-07-29T00:00:00Z', updatedAt: '2026-07-29T00:00:00Z' }],
    })
    renderPage()

    await fireEvent.update(await screen.findByRole('searchbox', { name: 'Search repositories' }), 'api')
    await fireEvent.click(await screen.findByText('api'))

    expect(mocks.routerPush).toHaveBeenCalledWith({ name: 'repository-detail', params: { project: 'payments', repositoryId: 'repository-1' } })
  })

  it('shows an error state when the search fails', async () => {
    mocks.searchRepositories.mockRejectedValue(new APIError(500, 'internal_error', 'Repository search failed'))
    renderPage()

    await fireEvent.update(await screen.findByRole('searchbox', { name: 'Search repositories' }), 'api')

    expect(await screen.findByText('Could not load results')).toBeTruthy()
    expect(screen.getByText('Repository search failed')).toBeTruthy()
  })
})
