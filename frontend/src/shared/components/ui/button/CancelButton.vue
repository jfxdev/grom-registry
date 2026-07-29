<script setup lang="ts">
import { X } from '@lucide/vue'
import Button from './Button.vue'

withDefaults(defineProps<{
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
}>(), {
  type: 'button',
  disabled: false,
})
</script>

<template>
  <Button variant="secondary" :type="type" :disabled="disabled">
    <X :size="17" aria-hidden="true" />
    <slot>Cancel</slot>
  </Button>
</template>
