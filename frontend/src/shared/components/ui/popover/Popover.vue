<script setup lang="ts">
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { ref } from 'vue'

defineProps<{ ariaLabel?: string }>()
const open = ref(false)

function close() {
  open.value = false
}

defineExpose({ close })
</script>

<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger as-child>
      <slot name="trigger" :open="open" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        class="popover-content"
        :aria-label="ariaLabel"
        side="bottom"
        align="start"
        :side-offset="6"
      >
        <slot :close="close" />
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<style scoped>
.popover-content {
  z-index: 50;
  box-shadow: 0 2px 0 rgba(0, 0, 0, 0.5), 0 10px 20px rgba(0, 0, 0, 0.22);
}
</style>
