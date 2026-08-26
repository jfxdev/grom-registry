<script setup lang="ts">
import Button from './Button.vue'

withDefaults(defineProps<{
  variant?: 'default' | 'cyan'
  type?: 'button' | 'submit' | 'reset'
  size?: 'default' | 'sm' | 'icon'
  disabled?: boolean
  loading?: boolean
}>(), {
  variant: 'default',
  type: 'button',
  size: 'default',
  disabled: false,
  loading: false,
})
</script>

<template>
  <Button
    :variant="variant"
    :type="type"
    :size="size"
    :disabled="disabled"
    :loading="loading"
  >
    <slot />
  </Button>
</template>
