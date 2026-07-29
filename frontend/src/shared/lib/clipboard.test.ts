// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { writeClipboardText } from './clipboard'

describe('writeClipboardText', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('reports unavailable clipboard access', async () => {
    vi.stubGlobal('navigator', {})
    await expect(writeClipboardText('secret')).resolves.toBe('unavailable')
  })

  it('reports clipboard failures', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    await expect(writeClipboardText('secret')).resolves.toBe('failed')
  })

  it('preserves successful copy feedback', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await expect(writeClipboardText('secret')).resolves.toBe('copied')
    expect(writeText).toHaveBeenCalledWith('secret')
  })
})
