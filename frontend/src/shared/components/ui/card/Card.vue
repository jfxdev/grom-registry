<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
defineProps<{ class?: string }>()
</script>

<template>
  <section :class="cn('rounded-lg border border-border bg-card p-5 shadow-sm', $props.class)">
    <slot />
  </section>
</template>
