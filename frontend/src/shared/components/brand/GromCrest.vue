<script setup lang="ts">
import crestUrl from '@/assets/logos/grom-crest.png'

withDefaults(defineProps<{
  size?: number
  alt?: string
}>(), {
  size: 48,
  alt: '',
})
</script>

<template>
  <img
    class="grom-crest"
    :src="crestUrl"
    :width="size"
    :height="size"
    :alt="alt"
    :aria-hidden="alt ? undefined : 'true'"
  />
</template>

<style scoped>
.grom-crest {
  display: block;
  flex: none;
  object-fit: contain;
}
</style>
