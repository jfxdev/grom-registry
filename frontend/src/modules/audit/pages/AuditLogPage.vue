<script setup lang="ts">
import type { AuditAction, AuditEvent, AuditResourceKind } from '@/shared/api/models'
import { Button, DeleteButton } from '@/shared/components/ui/button'
import { ButtonGroup } from '@/shared/components/ui/button-group'
import { Calendar } from '@/shared/components/ui/calendar'
import { Dialog } from '@/shared/components/ui/dialog'
import { Input } from '@/shared/components/ui/input'
import { PaginationControls } from '@/shared/components/ui/pagination'
import { Popover } from '@/shared/components/ui/popover'
import { PrincipalTypeBadge } from '@/shared/components/ui/principal-type-badge'
import { Select } from '@/shared/components/ui/select'
import { pageItems, useCursorPagination } from '@/shared/lib/pagination'
import type { DateValue } from '@internationalized/date'
import { getLocalTimeZone, today } from '@internationalized/date'
import { useQuery } from '@tanstack/vue-query'
import { CalendarDays, ScrollText, Search, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { AUDIT_ACTIONS, AUDIT_RESOURCE_KINDS, auditEventKeys, listAuditEvents, type AuditFilters } from '../api/auditEvents'

const pagination = useCursorPagination()
const actionFilter = ref<AuditAction | ''>('')
const resourceFilter = ref<AuditResourceKind | ''>('')
const actorQuery = ref('')
const fromDate = ref<DateValue>()
const toDate = ref<DateValue>()
const calendarPlaceholder = today(getLocalTimeZone())
const auditActionOptions: { value: AuditAction | ''; label: string }[] = [
  { value: '', label: 'All actions' },
  ...AUDIT_ACTIONS.map((action) => ({ value: action, label: action })),
]
const auditResourceOptions: { value: AuditResourceKind | ''; label: string }[] = [
  { value: '', label: 'All resources' },
  ...AUDIT_RESOURCE_KINDS.map((resource) => ({ value: resource, label: resource })),
]

function formatDate(date: DateValue): string {
  return date.toDate(getLocalTimeZone()).toLocaleDateString()
}

// The API expects RFC3339 bounds: `from` is inclusive, `to` is exclusive, so an
// end date is advanced to the start of the following day to include it fully.
// Bounds are computed in UTC regardless of the viewer's local time zone so the
// selected calendar day maps to a stable day boundary.
function startOfDay(date: DateValue | undefined): string | undefined {
  if (!date) return undefined
  return new Date(Date.UTC(date.year, date.month - 1, date.day)).toISOString()
}
function endOfDayExclusive(date: DateValue | undefined): string | undefined {
  if (!date) return undefined
  return new Date(Date.UTC(date.year, date.month - 1, date.day + 1)).toISOString()
}

const filters = computed<AuditFilters>(() => ({
  action: actionFilter.value || undefined,
  resource: resourceFilter.value || undefined,
  actor: actorQuery.value.trim() || undefined,
  from: startOfDay(fromDate.value),
  to: endOfDayExclusive(toDate.value),
}))

const events = useQuery({
  queryKey: computed(() => auditEventKeys.list(filters.value, pagination.cursor.value)),
  queryFn: () => listAuditEvents(filters.value, pagination.cursor.value),
})

const rows = computed(() => pageItems(events.data.value))
const currentPageCount = computed(() => rows.value.length)

watch([actionFilter, resourceFilter, actorQuery, fromDate, toDate], () => pagination.reset())

function actorKind(event: AuditEvent): 'user' | 'service_account' {
  return event.actorKind === 'service_account' ? 'service_account' : 'user'
}

function actorLabel(event: AuditEvent): string {
  return event.actorName || event.actorUsername || event.actorId || 'System'
}

function metadataSummary(event: AuditEvent): string {
  const metadata = event.metadata as Record<string, unknown> | undefined
  if (!metadata || Object.keys(metadata).length === 0) return '—'
  return JSON.stringify(metadata)
}

const selectedEvent = ref<AuditEvent | null>(null)

function hasMetadata(event: AuditEvent): boolean {
  const metadata = event.metadata as Record<string, unknown> | undefined
  return Boolean(metadata && Object.keys(metadata).length > 0)
}

function metadataDetail(event: AuditEvent): string {
  return JSON.stringify(event.metadata ?? {}, null, 2)
}
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Accountability</p>
        <h1 class="page-title">Audit log</h1>
        <p class="page-description">A read-only record of who did what across the installation. Events are immutable.</p>
      </div>
    </header>

    <div class="table-shell">
      <div class="list-toolbar audit-toolbar">
        <div>
          <h2>Events</h2>
          <p>Filter by action, resource, actor, or time range.</p>
        </div>
        <div class="audit-filters">
          <div class="list-search">
            <Search :size="16" aria-hidden="true" />
            <Input v-model="actorQuery" type="search" placeholder="Search actor" aria-label="Search by actor" />
          </div>
          <Select v-model="resourceFilter" :options="auditResourceOptions" ariaLabel="Filter by resource type" />
          <Select v-model="actionFilter" :options="auditActionOptions" ariaLabel="Filter by action" />
          <div class="audit-date-field">
            <ButtonGroup aria-label="From date filter">
              <Popover aria-label="Choose a from date">
                <template #trigger="{ open }">
                  <Button type="button" size="sm" variant="outline" aria-label="Choose from date" :aria-expanded="open">
                    <CalendarDays :size="14" aria-hidden="true" />
                    <span>{{ fromDate ? formatDate(fromDate) : 'From date' }}</span>
                  </Button>
                </template>
                <template #default="{ close }">
                  <Calendar
                    :model-value="fromDate"
                    :default-placeholder="fromDate ?? calendarPlaceholder"
                    @update:model-value="(value) => { fromDate = value; close() }"
                  />
                </template>
              </Popover>
              <DeleteButton v-if="fromDate" type="button" size="icon" aria-label="Clear from date" @click="fromDate = undefined" />
            </ButtonGroup>
          </div>
          <div class="audit-date-field">
            <ButtonGroup aria-label="To date filter">
              <Popover aria-label="Choose a to date">
                <template #trigger="{ open }">
                  <Button type="button" size="sm" variant="outline" aria-label="Choose to date" :aria-expanded="open">
                    <CalendarDays :size="14" aria-hidden="true" />
                    <span>{{ toDate ? formatDate(toDate) : 'To date' }}</span>
                  </Button>
                </template>
                <template #default="{ close }">
                  <Calendar
                    :model-value="toDate"
                    :default-placeholder="toDate ?? fromDate ?? calendarPlaceholder"
                    :min-value="fromDate"
                    @update:model-value="(value) => { toDate = value; close() }"
                  />
                </template>
              </Popover>
              <DeleteButton v-if="toDate" type="button" size="icon" aria-label="Clear to date" @click="toDate = undefined" />
            </ButtonGroup>
          </div>
        </div>
      </div>

      <div v-if="events.isLoading.value" class="empty-state list-empty-state" role="status">
        <p class="text-sm text-muted-foreground">Loading events…</p>
      </div>
      <div v-else-if="events.isError.value" class="empty-state list-empty-state" role="alert">
        <div>
          <p class="font-medium text-foreground">Unable to load audit events</p>
          <p class="mt-1 text-sm">Please try again.</p>
          <Button class="mt-4" variant="outline" :disabled="events.isFetching.value" @click="events.refetch()">Retry</Button>
        </div>
      </div>
      <div v-else-if="!currentPageCount" class="empty-state list-empty-state">
        <div>
          <ScrollText class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No audit events</p>
          <p class="mt-1 text-sm">No events match the current filters.</p>
        </div>
      </div>
      <template v-else>
        <div class="audit-table-head" aria-hidden="true">
          <span>Time</span><span>Actor</span><span>Action</span><span>Resource</span><span>Details</span>
        </div>
        <div
          v-for="event in rows"
          :key="event.id"
          class="data-row audit-row"
          role="button"
          tabindex="0"
          :aria-label="`View details for ${event.action}`"
          @click="selectedEvent = event"
          @keydown.enter.prevent="selectedEvent = event"
          @keydown.space.prevent="selectedEvent = event"
        >
          <p class="audit-time text-xs text-muted-foreground">{{ new Date(event.createdAt).toLocaleString() }}</p>
          <div class="audit-actor">
            <PrincipalTypeBadge :kind="actorKind(event)" icon-only />
            <span class="text-sm font-medium">{{ actorLabel(event) }}</span>
          </div>
          <p class="audit-action text-sm"><code>{{ event.action }}</code></p>
          <p class="audit-resource text-sm text-muted-foreground">
            {{ event.resourceKind }}<span v-if="event.resourceId" class="audit-resource-id"> · {{ event.resourceId }}</span>
          </p>
          <p class="audit-details text-xs text-muted-foreground"><code>{{ metadataSummary(event) }}</code></p>
        </div>
      </template>

      <div class="list-pagination">
        <PaginationControls
          :page="pagination.page.value"
          :has-previous="pagination.hasPrevious.value"
          :has-next="Boolean(events.data.value?.nextCursor)"
          :disabled="events.isFetching.value"
          @previous="pagination.previous()"
          @next="pagination.next(events.data.value?.nextCursor)"
        />
      </div>
    </div>

    <Dialog v-if="selectedEvent" labelled-by="audit-event-title" @close="selectedEvent = null">
      <section class="modal audit-detail form-stack">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="eyebrow">Audit event</p>
            <h2 id="audit-event-title" class="text-lg font-semibold"><code>{{ selectedEvent.action }}</code></h2>
            <p class="mt-1 text-xs text-muted-foreground">{{ new Date(selectedEvent.createdAt).toLocaleString() }}</p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close event details" @click="selectedEvent = null">
            <X :size="18" />
          </Button>
        </div>

        <dl class="audit-detail-grid">
          <dt>Actor</dt>
          <dd class="audit-actor">
            <PrincipalTypeBadge :kind="actorKind(selectedEvent)" icon-only />
            <span>{{ actorLabel(selectedEvent) }}</span>
          </dd>
          <dt>Actor kind</dt>
          <dd>{{ selectedEvent.actorKind }}</dd>
          <dt>Actor id</dt>
          <dd><code>{{ selectedEvent.actorId || '—' }}</code></dd>
          <dt>Resource kind</dt>
          <dd>{{ selectedEvent.resourceKind }}</dd>
          <dt>Resource id</dt>
          <dd><code>{{ selectedEvent.resourceId || '—' }}</code></dd>
          <dt>Event id</dt>
          <dd><code>{{ selectedEvent.id }}</code></dd>
        </dl>

        <div class="audit-metadata">
          <p class="audit-metadata-label">Metadata</p>
          <pre v-if="hasMetadata(selectedEvent)" class="audit-metadata-value"><code>{{ metadataDetail(selectedEvent) }}</code></pre>
          <p v-else class="text-sm text-muted-foreground">No metadata recorded for this event.</p>
        </div>

        <div class="flex justify-end">
          <Button variant="outline" @click="selectedEvent = null">Close</Button>
        </div>
      </section>
    </Dialog>
  </div>
</template>

<style scoped>
.list-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid var(--border);
  padding: 0.9rem 1rem;
}

.list-toolbar h2 {
  margin: 0;
  font-size: 0.92rem;
  font-weight: 650;
}

.list-toolbar p {
  margin: 0.3rem 0 0;
  color: var(--muted-foreground);
  font-size: 0.75rem;
}

.audit-toolbar {
  align-items: flex-start;
  flex-wrap: wrap;
}

.audit-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.65rem;
}

.audit-date-field {
  display: flex;
  align-items: center;
}

.list-search {
  position: relative;
  width: min(14rem, 34vw);
}

.list-search > svg {
  position: absolute;
  top: 50%;
  left: 0.8rem;
  z-index: 2;
  color: var(--muted-foreground);
  pointer-events: none;
  transform: translateY(-50%);
}

.list-search :deep(.grom-input) {
  min-height: 2.35rem;
  padding-left: 2.35rem;
}

.list-empty-state {
  min-height: 16rem;
  border: 0;
  border-radius: 0;
}

.list-pagination {
  border-top: 1px solid var(--border);
}

.audit-table-head,
.audit-row {
  grid-template-columns: 11rem minmax(9rem, 1fr) minmax(12rem, 1.2fr) minmax(9rem, 1fr) minmax(12rem, 1.4fr);
}

.audit-table-head {
  display: grid;
  align-items: center;
  gap: 1rem;
  border-bottom: 1px solid var(--border);
  padding: 0.55rem 1rem;
  color: var(--muted-foreground);
  font-size: 0.67rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.audit-row {
  gap: 1rem;
  align-items: center;
  cursor: pointer;
}

.audit-row:hover {
  background: var(--muted, rgba(127, 127, 127, 0.06));
}

.audit-row:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.audit-time {
  white-space: nowrap;
}

.audit-actor {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.audit-action code,
.audit-details code {
  overflow-wrap: anywhere;
}

.audit-details {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audit-resource-id {
  overflow-wrap: anywhere;
}

@media (max-width: 1100px) {
  .audit-table-head { display: none; }
  .audit-row {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    row-gap: 0.4rem;
  }
  .audit-details { grid-column: 1 / span 2; white-space: normal; }
}

.audit-detail {
  width: min(100%, 40rem);
  max-height: calc(100dvh - 2rem);
  overflow-y: auto;
}

.audit-detail-grid {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 0.55rem 1rem;
  margin: 0;
  font-size: 0.85rem;
}

.audit-detail-grid dt {
  color: var(--muted-foreground);
  font-size: 0.72rem;
  font-weight: 650;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.audit-detail-grid dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.audit-metadata-label {
  margin: 0 0 0.4rem;
  color: var(--muted-foreground);
  font-size: 0.72rem;
  font-weight: 650;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.audit-metadata-value {
  max-width: 100%;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 0.55rem;
  background: rgba(0, 0, 0, 0.22);
  padding: 0.8rem;
  margin: 0;
}

.audit-metadata-value code {
  display: block;
  font-size: 0.78rem;
  line-height: 1.6;
  white-space: pre;
}
</style>
