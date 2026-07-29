<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
defineProps<{ tone?: 'neutral' | 'success' | 'warning' | 'danger'; class?: string }>()
</script>

<template>
  <span :class="cn(
    'inline-flex items-center rounded-full border px-2 py-0.5 text-[0.68rem] font-semibold uppercase tracking-wider',
    tone === 'success' && 'border-accent/25 bg-accent/10 text-[#c9df6c]',
    tone === 'warning' && 'border-warning/25 bg-warning/10 text-[#e3c17d]',
    tone === 'danger' && 'border-danger/25 bg-danger/10 text-[#f2a09b]',
    (!tone || tone === 'neutral') && 'border-border bg-foreground/[.035] text-muted-foreground',
    $props.class,
  )">
    <slot />
  </span>
</template>
