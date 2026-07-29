<script setup lang="ts">
import Button from './Button.vue'

withDefaults(defineProps<{
  type?: 'button' | 'submit' | 'reset'
  size?: 'default' | 'sm' | 'icon'
  disabled?: boolean
  loading?: boolean
}>(), {
  type: 'button',
  size: 'default',
  disabled: false,
  loading: false,
})
</script>

<template>
  <Button
    variant="default"
    :type="type"
    :size="size"
    :disabled="disabled"
    :loading="loading"
  >
    <slot />
  </Button>
</template>
