export type ClipboardWriteResult = 'copied' | 'unavailable' | 'failed'

export async function writeClipboardText(text: string): Promise<ClipboardWriteResult> {
  if (typeof navigator === 'undefined') return 'unavailable'
  const writeText = navigator.clipboard?.writeText?.bind(navigator.clipboard)
  if (!writeText) return 'unavailable'
  try {
    await writeText(text)
    return 'copied'
  } catch {
    return 'failed'
  }
}
