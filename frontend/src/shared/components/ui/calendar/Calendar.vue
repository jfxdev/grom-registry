<script setup lang="ts">
import type { DateValue } from '@internationalized/date'
import {
  CalendarCell,
  CalendarCellTrigger,
  CalendarGrid,
  CalendarGridBody,
  CalendarGridHead,
  CalendarGridRow,
  CalendarHeadCell,
  CalendarHeader,
  CalendarHeading,
  CalendarNext,
  CalendarPrev,
  CalendarRoot,
} from 'reka-ui'
import { ChevronLeft, ChevronRight } from '@lucide/vue'

defineProps<{
  modelValue?: DateValue
  defaultPlaceholder: DateValue
  minValue?: DateValue
  maxValue?: DateValue
}>()

defineEmits<{ 'update:modelValue': [value: DateValue | undefined] }>()
</script>

<template>
  <CalendarRoot
    v-slot="{ weekDays, grid }"
    class="grom-calendar"
    :model-value="modelValue"
    :default-placeholder="defaultPlaceholder"
    :min-value="minValue"
    :max-value="maxValue"
    fixed-weeks
    initial-focus
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <CalendarHeader class="calendar-header">
      <CalendarPrev class="calendar-nav" aria-label="Previous month"><ChevronLeft :size="16" /></CalendarPrev>
      <CalendarHeading class="calendar-heading" />
      <CalendarNext class="calendar-nav" aria-label="Next month"><ChevronRight :size="16" /></CalendarNext>
    </CalendarHeader>
    <CalendarGrid v-for="month in grid" :key="month.value.toString()" class="calendar-grid">
      <CalendarGridHead>
        <CalendarGridRow>
          <CalendarHeadCell v-for="day in weekDays" :key="day" class="calendar-weekday">{{ day }}</CalendarHeadCell>
        </CalendarGridRow>
      </CalendarGridHead>
      <CalendarGridBody>
        <CalendarGridRow v-for="week in month.rows" :key="week[0]?.toString()">
          <CalendarCell v-for="day in week" :key="day.toString()" :date="day" class="calendar-cell">
            <CalendarCellTrigger v-slot="{ dayValue, disabled, selected, today, outsideView }" :day="day" :month="month.value" as-child>
              <button
                type="button"
                class="calendar-day"
                :class="{ selected, today, 'outside-view': outsideView }"
                :disabled="disabled"
              >
                {{ dayValue }}
              </button>
            </CalendarCellTrigger>
          </CalendarCell>
        </CalendarGridRow>
      </CalendarGridBody>
    </CalendarGrid>
  </CalendarRoot>
</template>

<style scoped>
.grom-calendar { padding: .75rem; border: 1px solid var(--border); border-radius: .7rem; background: var(--surface); user-select: none; -webkit-user-select: none; }
.calendar-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: .55rem; }
.calendar-heading { color: var(--foreground); font-size: .82rem; font-weight: 650; }
.calendar-nav, .calendar-day { border: 0; border-radius: .45rem; background: transparent; color: var(--foreground); cursor: pointer; }
.calendar-nav { display: grid; width: 2rem; height: 2rem; place-items: center; }
.calendar-nav:hover, .calendar-day:hover:not(:disabled) { background: var(--surface-raised); }
.calendar-grid { width: 100%; border-collapse: collapse; }
.calendar-weekday { height: 1.8rem; color: var(--muted-foreground); font-size: .68rem; font-weight: 650; text-align: center; }
.calendar-cell { padding: .08rem; text-align: center; }
.calendar-day { width: 2rem; height: 2rem; font-size: .75rem; }
.calendar-day.today { outline: 1px solid color-mix(in srgb, var(--accent) 70%, transparent); outline-offset: -1px; }
.calendar-day.selected { background: var(--accent); color: var(--accent-foreground); font-weight: 700; }
.calendar-day.outside-view { color: var(--muted-foreground); opacity: .5; }
.calendar-day:disabled { cursor: not-allowed; opacity: .32; }
.calendar-nav:focus-visible, .calendar-day:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
</style>
