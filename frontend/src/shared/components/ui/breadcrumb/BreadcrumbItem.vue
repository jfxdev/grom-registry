<script setup lang="ts">
import { cn } from '@/shared/lib/cn'

defineProps<{ class?: string }>()
</script>

<template>
  <li :class="cn('inline-flex items-center gap-1.5', $props.class)">
    <slot />
  </li>
</template>
