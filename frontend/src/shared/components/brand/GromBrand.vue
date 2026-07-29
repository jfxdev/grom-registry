<script setup lang="ts">
import GromCrest from './GromCrest.vue'

withDefaults(defineProps<{
  layout?: 'horizontal' | 'stacked'
  crestSize?: number
  compact?: boolean
}>(), {
  layout: 'horizontal',
  crestSize: 42,
  compact: false,
})
</script>

<template>
  <div class="grom-brand" :class="[`grom-brand-${layout}`, { 'grom-brand-compact': compact }]">
    <GromCrest :size="crestSize" alt="" />
    <div class="grom-brand-copy">
      <span class="grom-brand-name">Grom</span>
      <span v-if="!compact" class="grom-brand-product">Registry</span>
    </div>
  </div>
</template>

<style scoped>
.grom-brand {
  display: inline-flex;
  align-items: center;
  color: var(--foreground);
}

.grom-brand-horizontal {
  gap: 0.7rem;
}

.grom-brand-stacked {
  flex-direction: column;
  gap: 0.85rem;
  text-align: center;
}

.grom-brand-copy {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  font-weight: 760;
  letter-spacing: -0.045em;
  line-height: 1;
}

.grom-brand-stacked .grom-brand-copy {
  font-size: clamp(1.5rem, 4vw, 1.9rem);
}

.grom-brand-product {
  color: var(--muted-foreground);
  font-weight: 560;
}

.grom-brand-compact .grom-brand-copy {
  font-size: 1.15rem;
}
</style>
