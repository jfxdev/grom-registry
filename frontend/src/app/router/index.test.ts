// @vitest-environment jsdom

import { useSessionStore } from '@/modules/auth/store/session'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { router } from './index'

describe('application router', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('redirects signed-in users away from password reset links', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d',
      email: 'alex@example.com',
      username: 'alex',
      systemAdmin: false,
      systemViewer: false,
      createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/reset-password#token=grmpr_public_secret')

    expect(router.currentRoute.value.name).toBe('profile')
  })

  it('redirects unknown paths to projects', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d',
      email: 'alex@example.com',
      username: 'alex',
      systemAdmin: false,
      systemViewer: false,
      createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/not-a-real-page')

    expect(router.currentRoute.value.name).toBe('projects')
  })

  it('keeps non-administrators away from the audit log', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d', email: 'alex@example.com', username: 'alex',
      systemAdmin: false, systemViewer: true, createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/audit-log')

    expect(router.currentRoute.value.name).toBe('projects')
  })

  it('resolves the audit log for administrators', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d', email: 'alex@example.com', username: 'alex',
      systemAdmin: true, systemViewer: false, createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/audit-log')

    expect(router.currentRoute.value.name).toBe('audit-log')
  })

  it('keeps non-administrators away from repository search', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d', email: 'alex@example.com', username: 'alex',
      systemAdmin: false, systemViewer: true, createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/repository-search')

    expect(router.currentRoute.value.name).toBe('projects')
  })

  it('resolves repository search for administrators', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d', email: 'alex@example.com', username: 'alex',
      systemAdmin: true, systemViewer: false, createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/repository-search')

    expect(router.currentRoute.value.name).toBe('repository-search')
  })

  it('resolves a dedicated repository page', async () => {
    const session = useSessionStore()
    session.checked = true
    session.user = {
      id: '8a24b252-3aa7-4cc7-8384-52441dab9f1d', email: 'alex@example.com', username: 'alex',
      systemAdmin: false, systemViewer: false, createdAt: '2026-07-27T00:00:00Z',
    }

    await router.push('/projects/payments/repositories/repository-1')

    expect(router.currentRoute.value.name).toBe('repository-detail')
  })
})
