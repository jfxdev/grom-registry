<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
import type { HTMLAttributes } from 'vue'

defineOptions({ inheritAttrs: false })

defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div v-bind="$attrs" role="group" :class="cn('button-group', $props.class)">
    <slot />
  </div>
</template>

<style scoped>
.button-group {
  display: inline-flex;
  align-items: stretch;
  isolation: isolate;
}

.button-group :deep(.grom-button) {
  border-radius: 0;
}

.button-group :deep(.grom-button + .grom-button) {
  margin-left: -1px;
}

.button-group :deep(.grom-button:first-child) {
  border-top-left-radius: 0.65rem;
  border-bottom-left-radius: 0.65rem;
}

.button-group :deep(.grom-button:last-child) {
  border-top-right-radius: 0.65rem;
  border-bottom-right-radius: 0.65rem;
}

.button-group :deep(.grom-button:focus-visible) {
  z-index: 1;
}
</style>
