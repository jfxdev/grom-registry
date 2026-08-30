<script setup lang="ts">
import { ChevronDown } from '@lucide/vue'
import { ref } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  defaultOpen?: boolean
}>(), {
  description: '',
  defaultOpen: false,
})

const open = ref(props.defaultOpen)
const contentId = `accordion-${Math.random().toString(36).slice(2)}`
</script>

<template>
  <section class="accordion" :class="{ open }">
    <button
      class="accordion-trigger"
      type="button"
      :aria-expanded="open"
      :aria-controls="contentId"
      @click="open = !open"
    >
      <span class="min-w-0 flex-1 text-left">
        <span class="accordion-title">{{ title }}</span>
        <span v-if="description" class="accordion-description">{{ description }}</span>
      </span>
      <ChevronDown class="accordion-chevron" :class="{ open }" :size="17" aria-hidden="true" />
    </button>
    <div v-show="open" :id="contentId" class="accordion-content">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.accordion {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: .7rem;
  background: var(--card);
}

.accordion-trigger {
  display: flex;
  width: 100%;
  align-items: center;
  gap: .75rem;
  border: 0;
  background: transparent;
  padding: .85rem 1rem;
  color: var(--foreground);
  cursor: pointer;
}

.accordion-trigger:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: -2px; }
.accordion-title, .accordion-description { display: block; }
.accordion-title { font-size: .84rem; font-weight: 700; }
.accordion-description { margin-top: .16rem; color: var(--muted-foreground); font-size: .74rem; line-height: 1.4; }
.accordion-chevron { flex: none; color: var(--muted-foreground); transition: transform var(--motion-fast) ease; }
.accordion-chevron.open { transform: rotate(180deg); }
.accordion-content { border-top: 1px solid var(--border); padding: 1rem; display: grid; gap: .75rem; }
</style>
