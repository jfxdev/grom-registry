<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
import { useSlots } from 'vue'

defineOptions({ inheritAttrs: false })

defineProps<{
  modelValue?: string
  type?: string
  placeholder?: string
  autocomplete?: string
  required?: boolean
  disabled?: boolean
  readonly?: boolean
  class?: string
}>()

defineEmits<{ 'update:modelValue': [value: string] }>()

const slots = useSlots()
</script>

<template>
  <div class="input-shell" :class="{ 'input-shell-trailing': slots.trailing }">
    <input
      v-bind="$attrs"
      :value="modelValue"
      :type="type ?? 'text'"
      :placeholder="placeholder"
      :autocomplete="autocomplete"
      :required="required"
      :disabled="disabled"
      :readonly="readonly"
      :class="cn('grom-input', $props.class)"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <div v-if="slots.trailing" class="input-trailing">
      <slot name="trailing" />
    </div>
  </div>
</template>

<style scoped>
.input-shell {
  position: relative;
  width: 100%;
  border-radius: 0.65rem;
}

.input-shell::after {
  position: absolute;
  inset: 0;
  border: 1px solid transparent;
  border-radius: inherit;
  content: "";
  pointer-events: none;
  transition: border-color var(--motion-standard) ease, box-shadow var(--motion-standard) ease;
}

.input-shell:focus-within::after {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--focus-ring) 18%, transparent);
}

.grom-input {
  width: 100%;
  min-width: 0;
  min-height: 2.75rem;
  border: 1px solid var(--border);
  border-radius: inherit;
  background: rgba(0, 0, 0, 0.18);
  padding: 0 0.85rem;
  color: var(--foreground);
  font: inherit;
  font-size: 0.875rem;
  transition: border-color var(--motion-standard) ease, background var(--motion-standard) ease;
}

.grom-input::placeholder {
  color: color-mix(in srgb, var(--muted-foreground) 68%, transparent);
}

.grom-input:focus {
  border-color: transparent;
  background: rgba(0, 0, 0, 0.24);
  outline: none;
}

.grom-input[aria-invalid="true"] {
  border-color: color-mix(in srgb, var(--danger) 72%, transparent);
}

.grom-input:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.input-shell-trailing .grom-input {
  padding-right: 3.2rem;
}

.input-trailing {
  position: absolute;
  top: 50%;
  right: 0.35rem;
  z-index: 1;
  display: grid;
  place-items: center;
  transform: translateY(-50%);
}

@media (max-width: 600px) {
  .grom-input {
    min-height: 3rem;
    font-size: 1rem;
  }
}
</style>
