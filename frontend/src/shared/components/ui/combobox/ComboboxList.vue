<script setup lang="ts">
import type { ComboboxContentEmits, ComboboxContentProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { ComboboxContent, ComboboxPortal, ComboboxViewport, useForwardPropsEmits } from "reka-ui"
import { cn } from '@/shared/lib/cn'

const props = withDefaults(defineProps<ComboboxContentProps & {
  class?: HTMLAttributes["class"]
  disablePortal?: boolean
}>(), {
  position: "popper",
  align: "center",
  sideOffset: 4,
  disablePortal: false,
  class: undefined,
})
const emits = defineEmits<ComboboxContentEmits>()

const delegatedProps = reactiveOmit(props, "class", "disablePortal")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <ComboboxPortal :disabled="disablePortal">
    <ComboboxContent
      v-bind="forwarded"
      :class="cn('z-50 w-[200px] rounded-md border bg-popover text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2', props.class)"
    >
      <ComboboxViewport>
        <slot />
      </ComboboxViewport>
    </ComboboxContent>
  </ComboboxPortal>
</template>
