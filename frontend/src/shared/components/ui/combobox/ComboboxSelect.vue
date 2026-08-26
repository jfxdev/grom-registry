<script setup lang="ts" generic="T extends string | number">
import { Check, ChevronsUpDown } from '@lucide/vue'
import { computed } from 'vue'
import Combobox from './Combobox.vue'
import ComboboxAnchor from './ComboboxAnchor.vue'
import ComboboxEmpty from './ComboboxEmpty.vue'
import ComboboxGroup from './ComboboxGroup.vue'
import ComboboxInput from './ComboboxInput.vue'
import ComboboxItem from './ComboboxItem.vue'
import { ComboboxItemIndicator } from 'reka-ui'
import ComboboxList from './ComboboxList.vue'
import ComboboxTrigger from './ComboboxTrigger.vue'

type Option = { value: T; label: string }

const props = withDefaults(defineProps<{
  modelValue: T | null
  options: Option[]
  placeholder?: string
  emptyText?: string
  disabled?: boolean
}>(), {
  placeholder: 'Select an option…',
  emptyText: 'No matching option.',
  disabled: false,
})

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()

const model = computed({
  get: () => props.modelValue,
  set: (value: T) => emit('update:modelValue', value),
})

function displayLabel(value: unknown) {
  return props.options.find((option) => option.value === value)?.label ?? ''
}
</script>

<template>
  <Combobox
    v-model="model"
    :disabled="disabled"
    open-on-click
    open-on-focus
    reset-search-term-on-select
    class="combobox-select"
  >
    <ComboboxAnchor class="combobox-select-anchor">
      <ComboboxInput
        class="combobox-select-input"
        :display-value="displayLabel"
        :placeholder="placeholder"
      />
      <ComboboxTrigger class="combobox-select-trigger" aria-label="Toggle options">
        <ChevronsUpDown :size="15" />
      </ComboboxTrigger>
    </ComboboxAnchor>
    <ComboboxList class="combobox-select-list" disable-portal>
      <ComboboxEmpty>{{ emptyText }}</ComboboxEmpty>
      <ComboboxGroup>
        <ComboboxItem v-for="option in options" :key="String(option.value)" :value="option.value">
          {{ option.label }}
          <ComboboxItemIndicator>
            <Check :size="15" />
          </ComboboxItemIndicator>
        </ComboboxItem>
      </ComboboxGroup>
    </ComboboxList>
  </Combobox>
</template>

<style scoped>
.combobox-select {
  width: 100%;
}

.combobox-select-anchor {
  position: relative;
  width: 100%;
}

.combobox-select-input {
  width: 100%;
  height: auto;
  padding-right: 2.25rem;
}

.combobox-select-trigger {
  position: absolute;
  top: 50%;
  right: 0.5rem;
  display: grid;
  place-items: center;
  transform: translateY(-50%);
  color: var(--muted-foreground);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.combobox-select-list {
  width: var(--reka-combobox-trigger-width);
  max-height: 16rem;
  overflow: auto;
  padding: 0.25rem;
}
</style>
