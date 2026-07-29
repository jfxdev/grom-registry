<script setup lang="ts">
import { cn } from '@/shared/lib/cn'
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  variant?: 'default' | 'secondary' | 'outline' | 'ghost' | 'danger'
  size?: 'default' | 'sm' | 'icon'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  class?: string
}>(), {
  variant: 'default',
  size: 'default',
  type: 'button',
  loading: false,
  class: '',
})

const classes = computed(() => cn(
  'grom-button',
  `grom-button-variant-${props.variant}`,
  `grom-button-size-${props.size}`,
  props.class,
))
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
    :class="classes"
  >
    <slot />
  </button>
</template>

<style scoped>
.grom-button {
  --button-rise: 3px;
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  border: 1px solid transparent;
  border-radius: 0.65rem;
  font-weight: 680;
  line-height: 1;
  cursor: pointer;
  transform: translateY(0);
  transition:
    transform var(--motion-fast) ease-out,
    box-shadow var(--motion-fast) ease-out,
    background var(--motion-standard) ease,
    border-color var(--motion-standard) ease,
    color var(--motion-standard) ease;
}

.grom-button:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 3px;
}

.grom-button:active:not(:disabled) {
  transform: translateY(var(--button-rise));
}

.grom-button:disabled {
  cursor: not-allowed;
  filter: saturate(0.55);
  opacity: 0.58;
  transform: none;
  box-shadow: none;
}

.grom-button-variant-default {
  border-color: color-mix(in srgb, var(--button-highlight) 70%, var(--bronze));
  background: linear-gradient(180deg, var(--button-highlight) 0%, var(--accent) 18%, var(--accent-pressed) 100%);
  color: var(--accent-foreground);
  box-shadow:
    0 var(--button-rise) 0 var(--button-shadow),
    0 calc(var(--button-rise) + 4px) 12px rgba(0, 0, 0, 0.24),
    inset 0 1px 0 rgba(255, 255, 255, 0.2);
}

.grom-button-variant-default:hover:not(:disabled) {
  background: linear-gradient(180deg, #c0dc4c 0%, var(--accent-hover) 22%, var(--accent) 100%);
}

.grom-button-variant-default:active:not(:disabled) {
  box-shadow:
    0 0 0 var(--button-shadow),
    0 2px 5px rgba(0, 0, 0, 0.22),
    inset 0 1px 2px rgba(0, 0, 0, 0.18);
}

.grom-button-variant-secondary {
  border-color: #4f544f;
  background: linear-gradient(180deg, #515651 0%, #3c413c 18%, #2b302b 100%);
  color: var(--foreground);
  box-shadow:
    0 var(--button-rise) 0 #191d19,
    0 calc(var(--button-rise) + 4px) 12px rgba(0, 0, 0, 0.24),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}

.grom-button-variant-secondary:hover:not(:disabled) {
  background: linear-gradient(180deg, #606560 0%, #484d48 22%, #343934 100%);
}

.grom-button-variant-secondary:active:not(:disabled) {
  box-shadow:
    0 0 0 #191d19,
    0 2px 5px rgba(0, 0, 0, 0.22),
    inset 0 1px 2px rgba(0, 0, 0, 0.2);
}

.grom-button-variant-outline,
.grom-button-variant-danger {
  --button-rise: 2px;
  box-shadow:
    0 var(--button-rise) 0 rgba(0, 0, 0, 0.5),
    inset 0 1px 0 rgba(255, 255, 255, 0.035);
}

.grom-button-variant-outline {
  border-color: var(--border);
  background: linear-gradient(180deg, var(--surface-raised), var(--surface));
  color: var(--foreground);
}

.grom-button-variant-outline:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--bronze) 50%, var(--border));
  background: var(--surface-raised);
}

.grom-button-variant-danger {
  border-color: color-mix(in srgb, var(--danger) 32%, transparent);
  background: linear-gradient(180deg, color-mix(in srgb, var(--danger) 16%, var(--surface-raised)), color-mix(in srgb, var(--danger) 9%, var(--surface)));
  color: #f2a09b;
}

.grom-button-variant-danger:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--danger) 50%, transparent);
  background: color-mix(in srgb, var(--danger) 18%, var(--surface));
}

.grom-button-variant-outline:active:not(:disabled),
.grom-button-variant-danger:active:not(:disabled) {
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.18);
}

.grom-button-variant-ghost {
  --button-rise: 1px;
  border-color: transparent;
  background: transparent;
  color: var(--foreground);
  box-shadow: none;
}

.grom-button-variant-ghost:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.045);
}

.grom-button-variant-ghost:active:not(:disabled) {
  background: rgba(255, 255, 255, 0.065);
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.2);
}

.grom-button-size-default {
  min-height: 2.75rem;
  padding: 0 1rem;
  font-size: 0.875rem;
}

.grom-button-size-sm {
  --button-rise: 2px;
  min-height: 2.1rem;
  padding: 0 0.75rem;
  font-size: 0.75rem;
}

.grom-button-size-icon {
  --button-rise: 2px;
  width: 2.5rem;
  min-width: 2.5rem;
  height: 2.5rem;
  padding: 0;
}

@media (prefers-reduced-motion: reduce) {
  .grom-button {
    transition: none;
  }
}
</style>
