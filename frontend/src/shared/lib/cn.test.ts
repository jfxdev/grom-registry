import { describe, expect, it } from 'vitest'
import { cn } from './cn'

describe('cn', () => {
  it('merges conflicting utility classes', () => {
    expect(cn('px-2 text-sm', 'px-4')).toBe('text-sm px-4')
  })
})
