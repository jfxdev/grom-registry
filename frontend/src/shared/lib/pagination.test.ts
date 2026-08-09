// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { useCursorPagination } from './pagination'

describe('useCursorPagination', () => {
  it('moves between cursors and resets to the first page', () => {
    const pagination = useCursorPagination()

    expect(pagination.page.value).toBe(1)
    expect(pagination.cursor.value).toBe('')
    expect(pagination.hasPrevious.value).toBe(false)

    pagination.next('second')
    pagination.next('third')
    expect(pagination.page.value).toBe(3)
    expect(pagination.cursor.value).toBe('third')
    expect(pagination.hasPrevious.value).toBe(true)

    pagination.previous()
    expect(pagination.page.value).toBe(2)
    expect(pagination.cursor.value).toBe('second')

    pagination.reset()
    expect(pagination.page.value).toBe(1)
    expect(pagination.cursor.value).toBe('')
    expect(pagination.hasPrevious.value).toBe(false)
  })

  it('ignores empty next cursors and does not move before the first page', () => {
    const pagination = useCursorPagination()
    pagination.next(undefined)
    pagination.previous()

    expect(pagination.page.value).toBe(1)
    expect(pagination.cursor.value).toBe('')
  })
})
