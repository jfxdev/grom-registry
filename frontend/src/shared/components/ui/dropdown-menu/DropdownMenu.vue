<script setup lang="ts">
import { Button } from '@/shared/components/ui/button'
import { ChevronDown } from '@lucide/vue'
import { onBeforeUnmount, ref, watch } from 'vue'

defineProps<{ label: string; ariaLabel: string; disabled?: boolean }>()
const open = ref(false)
const root = ref<{ contains: (target: unknown) => boolean } | null>(null)
function close() { open.value = false }

function closeFromOutside(event: unknown) {
  const target = (event as { target?: unknown }).target
  if (root.value && !root.value.contains(target)) close()
}

function closeFromEscape(event: unknown) {
  if ((event as { key?: string }).key === 'Escape') close()
}

watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener('pointerdown', closeFromOutside)
    document.addEventListener('keydown', closeFromEscape)
    return
  }
  document.removeEventListener('pointerdown', closeFromOutside)
  document.removeEventListener('keydown', closeFromEscape)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', closeFromOutside)
  document.removeEventListener('keydown', closeFromEscape)
})
</script>

<template>
  <div ref="root" class="dropdown-menu">
    <Button size="sm" variant="outline" :disabled="disabled" :class="open ? 'dropdown-menu-trigger dropdown-menu-trigger-open' : 'dropdown-menu-trigger'" :aria-label="ariaLabel" :aria-expanded="open" aria-haspopup="menu" @click="open = !open">
      <slot name="icon" /><span>{{ label }}</span><ChevronDown :size="15" aria-hidden="true" />
    </Button>
    <div v-if="open" class="dropdown-menu-content" role="menu"><slot :close="close" /></div>
  </div>
</template>

<style scoped>
.dropdown-menu { position: relative; width: 100%; }
.dropdown-menu-trigger { width: 100%; justify-content: space-between; }
.dropdown-menu-trigger-open { border-bottom-color: transparent; border-radius: .65rem .65rem 0 0; box-shadow: none; }
.dropdown-menu-content { position: absolute; z-index: 10; top: 100%; right: 0; display: grid; min-width: 100%; overflow: hidden; border: 1px solid var(--border); border-radius: 0 0 .65rem .65rem; background: var(--surface); box-shadow: 0 2px 0 rgba(0, 0, 0, .5), 0 10px 20px rgba(0, 0, 0, .22); }
.dropdown-menu-content :deep(.dropdown-menu-item) { display: flex; align-items: center; gap: .5rem; border: 0; background: transparent; padding: .6rem .7rem; color: var(--foreground); font: inherit; font-size: .78rem; text-align: left; cursor: pointer; }
.dropdown-menu-content :deep(.dropdown-menu-item:hover), .dropdown-menu-content :deep(.dropdown-menu-item:focus-visible) { background: var(--muted); outline: none; }
</style>
