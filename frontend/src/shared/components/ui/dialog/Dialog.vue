<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{
  labelledBy: string
  restoreFocus?: globalThis.HTMLElement | null
}>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<{
  open: boolean
  showModal?: () => void
  close?: () => void
  setAttribute: (name: string, value: string) => void
} | null>(null)
const previousFocusedElement = ref<globalThis.HTMLElement | null>(null)

function requestClose() {
  emit('close')
}

function closeFromBackdrop(event: { target: unknown; currentTarget: unknown }) {
  if (event.target === event.currentTarget) requestClose()
}

onMounted(async () => {
  await nextTick()
  previousFocusedElement.value = document.activeElement instanceof globalThis.HTMLElement ? document.activeElement : null
  if (typeof dialog.value?.showModal === 'function') {
    dialog.value.showModal()
  } else {
    dialog.value?.setAttribute('open', '')
  }
})

onBeforeUnmount(() => {
  if (dialog.value?.open && typeof dialog.value.close === 'function') {
    dialog.value.close()
  }
  const focusTarget = props.restoreFocus ?? previousFocusedElement.value
  if (focusTarget?.isConnected) focusTarget.focus()
})
</script>

<template>
  <dialog
    ref="dialog"
    class="modal-backdrop dialog-root"
    :aria-labelledby="labelledBy"
    aria-modal="true"
    @cancel.prevent="requestClose"
    @click="closeFromBackdrop"
  >
    <slot />
  </dialog>
</template>

<style scoped>
.dialog-root {
  width: 100%;
  max-width: none;
  height: 100%;
  max-height: none;
  margin: 0;
  border: 0;
  color: inherit;
}

.dialog-root::backdrop {
  background: transparent;
}
</style>
