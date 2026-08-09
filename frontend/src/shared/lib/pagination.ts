export type Page<T> = { items: T[]; nextCursor?: string }

// Component mocks can remain concise arrays while callers migrate to Page<T>.
export function pageItems<T>(value: Page<T> | { tags?: T[] } | T[] | undefined | null): T[] {
  if (Array.isArray(value)) return value
  if (!value) return []
  if ('items' in value) return value.items
  return value.tags ?? []
}

export function useCursorPagination() {
  const cursors = ref([''])
  const cursor = computed(() => cursors.value[cursors.value.length - 1] ?? '')
  const page = computed(() => cursors.value.length)
  const previous = () => { if (cursors.value.length > 1) cursors.value.pop() }
  const next = (nextCursor: string | undefined) => { if (nextCursor) cursors.value.push(nextCursor) }
  const reset = () => { cursors.value = [''] }
  return { cursor, page, hasPrevious: computed(() => cursors.value.length > 1), previous, next, reset }
}
import { computed, ref } from 'vue'
