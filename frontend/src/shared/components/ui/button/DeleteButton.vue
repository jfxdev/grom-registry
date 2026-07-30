<script setup lang="ts">
import { Trash2 } from '@lucide/vue'
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
    variant="delete"
    :type="type"
    :size="size"
    :disabled="disabled"
    :loading="loading"
  >
    <Trash2 :size="17" aria-hidden="true" />
    <span v-if="size !== 'icon'"><slot>Delete</slot></span>
  </Button>
</template>
