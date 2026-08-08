<script setup lang="ts">
import { Button } from '@/shared/components/ui/button'
import { computed } from 'vue'

const props = defineProps<{
  page: number
  pageCount: number
  hasPrevious: boolean
  hasNext: boolean
  disabled?: boolean
}>()

const displayedPageCount = computed(() => Math.max(1, props.pageCount))

defineEmits<{ previous: []; next: [] }>()
</script>

<template>
  <nav class="pagination-controls" aria-label="Pagination">
    <Button variant="outline" size="sm" :disabled="disabled || !hasPrevious" @click="$emit('previous')">Previous</Button>
    <span class="pagination-status" aria-live="polite">Page {{ page }} of {{ displayedPageCount }}</span>
    <Button variant="outline" size="sm" :disabled="disabled || !hasNext" @click="$emit('next')">Next</Button>
  </nav>
</template>

<style scoped>
.pagination-controls { display: flex; align-items: center; justify-content: center; gap: .75rem; padding: .9rem 0; }
.pagination-status { color: var(--muted-foreground); font-size: .8rem; }
</style>
