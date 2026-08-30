<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
import type { HTMLAttributes } from 'vue'

defineProps<{ class?: HTMLAttributes['class'] }>()
</script>

<template>
  <section :class="cn('rounded-lg border border-border bg-card p-5 shadow-sm', $props.class)">
    <slot />
  </section>
</template>
