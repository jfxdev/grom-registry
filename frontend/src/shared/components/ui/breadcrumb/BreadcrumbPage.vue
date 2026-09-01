<script setup lang="ts">
import { cn } from '@/shared/lib/cn'

defineProps<{ class?: string }>()
</script>

<template>
  <span role="link" aria-disabled="true" aria-current="page" :class="cn('font-normal text-foreground', $props.class)">
    <slot />
  </span>
</template>
