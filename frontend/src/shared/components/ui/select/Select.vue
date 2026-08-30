<script setup lang="ts" generic="T extends string | number">
import { cn } from '@/shared/lib/cn'
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxItemIndicator,
  ComboboxList,
  ComboboxTrigger,
} from '@/shared/components/ui/combobox'
import { Check, ChevronsUpDown, Search } from '@lucide/vue'
import {
  computed,
  ref,
} from 'vue'
import type { HTMLAttributes } from 'vue'

type SelectOption<T> = {
  value: T
  label: string
}

const EMPTY_OPTION_VALUE = '__grom-select-empty-option__'

const props = withDefaults(defineProps<{
  modelValue: T
  options: SelectOption<T>[]
  ariaLabel: string
  disabled?: boolean
  portalTo?: string | globalThis.HTMLElement
  class?: HTMLAttributes['class']
}>(), {
  disabled: false,
  portalTo: undefined,
  class: undefined,
})

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()
const open = ref(false)
const searchTerm = ref('')

const model = computed<string | number>({
  get: () => props.modelValue === '' ? EMPTY_OPTION_VALUE : props.modelValue,
  set: (value) => emit('update:modelValue', (value === EMPTY_OPTION_VALUE ? '' : value) as T),
})

function itemValue(option: SelectOption<T>): string | number {
  return option.value === '' ? EMPTY_OPTION_VALUE : option.value
}

function displayLabel(value: unknown): string {
  const option = props.options.find((candidate) => itemValue(candidate) === value)
  return option?.label ?? ''
}

function setOpen(value: boolean) {
  open.value = value
  if (value) searchTerm.value = ''
}
</script>

<template>
  <Combobox
    v-model="model"
    :open="open"
    :disabled="disabled"
    open-on-click
    open-on-focus
    reset-search-term-on-select
    class="grom-select"
    @update:open="setOpen"
  >
    <ComboboxAnchor :class="cn('grom-select-anchor', $props.class)">
      <Search class="grom-select-search-icon" :size="15" aria-hidden="true" />
      <ComboboxInput
        v-model="searchTerm"
        class="grom-select-input"
        :display-value="displayLabel"
        :placeholder="`Search ${ariaLabel.toLowerCase()}…`"
        :aria-label="ariaLabel"
      />
      <ComboboxTrigger class="grom-select-trigger" :aria-label="`Open ${ariaLabel.toLowerCase()} options`">
        <ChevronsUpDown :size="16" aria-hidden="true" />
      </ComboboxTrigger>
    </ComboboxAnchor>
    <ComboboxList class="grom-select-content" :portal-to="portalTo">
      <ComboboxEmpty class="grom-select-empty">No options found.</ComboboxEmpty>
      <ComboboxGroup>
        <ComboboxItem v-for="option in options" :key="String(option.value)" :value="itemValue(option)" class="grom-select-item">
          <span class="grom-select-item-label" :title="option.label">{{ option.label }}</span>
          <ComboboxItemIndicator class="grom-select-indicator"><Check :size="15" /></ComboboxItemIndicator>
        </ComboboxItem>
      </ComboboxGroup>
    </ComboboxList>
  </Combobox>
</template>

<style>
.grom-select-anchor {
  position: relative;
  display: inline-flex;
  min-width: 10.5rem;
  min-height: 2.35rem;
  align-items: stretch;
}

.grom-select-input {
  width: 100%;
  min-width: 0;
  min-height: 2.35rem;
  border: 1px solid var(--border);
  border-radius: 0.65rem;
  background: linear-gradient(180deg, var(--surface-raised), var(--surface));
  padding: 0 2.35rem 0 2.2rem;
  color: var(--foreground);
  font: inherit;
  font-size: 0.78rem;
  text-align: left;
  cursor: pointer;
  box-shadow: 0 2px 0 rgba(0, 0, 0, 0.5), inset 0 1px 0 rgba(255, 255, 255, 0.035);
}

.grom-select-input:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--bronze) 50%, var(--border));
  background: var(--surface-raised);
}

.grom-select-input:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 3px;
}

.grom-select-input:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.grom-select-search-icon {
  position: absolute;
  top: 50%;
  left: 0.7rem;
  z-index: 1;
  color: var(--muted-foreground);
  pointer-events: none;
  transform: translateY(-50%);
}

.grom-select-trigger {
  position: absolute;
  top: 50%;
  right: 0.5rem;
  z-index: 1;
  display: grid;
  place-items: center;
  border: 0;
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transform: translateY(-50%);
}

.grom-select-content {
  z-index: 50;
  width: min(22rem, calc(100vw - 2rem)) !important;
  min-width: var(--reka-combobox-trigger-width);
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 0.65rem;
  background: var(--surface);
  color: var(--foreground);
  box-shadow: 0 2px 0 rgba(0, 0, 0, 0.5), 0 10px 20px rgba(0, 0, 0, 0.22);
}

.grom-select-empty {
  color: var(--muted-foreground);
}

.grom-select-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border-radius: 0.45rem;
  padding: 0.5rem 0.55rem;
  font-size: 0.78rem;
  outline: none;
  cursor: pointer;
}

.grom-select-item-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.grom-select-item[data-highlighted],
.grom-select-item:focus-visible {
  background: #2c3813;
  color: #faf8ef;
}

.grom-select-item[data-state='checked'] {
  background: color-mix(in srgb, var(--accent) 16%, var(--surface));
  color: #cce85c;
  font-weight: 650;
}

.grom-select-item[data-state='checked'][data-highlighted] {
  background: #3a4a16;
  color: #ffffff;
}

.grom-select-indicator {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
}
</style>
