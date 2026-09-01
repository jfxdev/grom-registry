<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
import { ChevronRight } from '@lucide/vue'

defineProps<{ class?: string }>()
</script>

<template>
  <li role="presentation" aria-hidden="true" :class="cn('[&>svg]:size-3.5', $props.class)">
    <slot>
      <ChevronRight :size="14" />
    </slot>
  </li>
</template>
