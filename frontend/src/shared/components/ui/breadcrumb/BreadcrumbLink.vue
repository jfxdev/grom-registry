<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
import type { Component } from 'vue'

const props = defineProps<{
  as?: string | Component
  class?: string
}>()
</script>

<template>
  <component :is="props.as ?? 'a'" :class="cn('transition-colors hover:text-foreground', $props.class)">
    <slot />
  </component>
</template>
