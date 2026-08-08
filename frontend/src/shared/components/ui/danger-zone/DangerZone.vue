<script setup lang="ts">
import { AlertTriangle, ChevronDown } from '@lucide/vue'
import { ref } from 'vue'

withDefaults(defineProps<{
  title?: string
  description?: string
}>(), {
  title: 'Danger zone',
  description: 'Actions here can have irreversible effects.',
})

const open = ref(false)
const contentId = `danger-zone-${Math.random().toString(36).slice(2)}`
</script>

<template>
  <section class="danger-zone">
    <button
      class="danger-zone-trigger"
      type="button"
      :aria-expanded="open"
      :aria-controls="contentId"
      @click="open = !open"
    >
      <span class="danger-zone-icon"><AlertTriangle :size="16" /></span>
      <span class="min-w-0 flex-1 text-left">
        <span class="danger-zone-title">{{ title }}</span>
        <span class="danger-zone-description">{{ description }}</span>
      </span>
      <ChevronDown class="danger-zone-chevron" :class="{ open }" :size="17" aria-hidden="true" />
    </button>
    <div v-if="open" :id="contentId" class="danger-zone-content">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.danger-zone {
  margin-top: 2rem;
  overflow: hidden;
  border: 1px solid #db6a65;
  border-radius: .7rem;
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--danger) 35%, transparent);
}

.danger-zone-trigger {
  display: flex;
  width: 100%;
  align-items: center;
  gap: .75rem;
  border: 0;
  background: color-mix(in srgb, var(--danger) 5%, transparent);
  padding: .9rem 1rem;
  color: var(--foreground);
  cursor: pointer;
}

.danger-zone-trigger:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: -2px; }
.danger-zone-icon { display: grid; flex: none; color: var(--danger); }
.danger-zone-title, .danger-zone-description { display: block; }
.danger-zone-title { color: var(--danger); font-size: .84rem; font-weight: 700; }
.danger-zone-description { margin-top: .16rem; color: var(--muted-foreground); font-size: .74rem; line-height: 1.4; }
.danger-zone-chevron { flex: none; color: var(--muted-foreground); transition: transform var(--motion-fast) ease; }
.danger-zone-chevron.open { transform: rotate(180deg); }
.danger-zone-content { border-top: 1px solid color-mix(in srgb, var(--danger) 40%, var(--border)); padding: 1rem; }
</style>
