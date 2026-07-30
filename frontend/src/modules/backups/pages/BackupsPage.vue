<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import type { BackupStatus, BackupSummary } from '@/shared/api/models'
import { Badge } from '@/shared/components/ui/badge'
import { ActionButton, Button, CancelButton, DeleteButton } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Archive, ChevronLeft, ChevronRight, CircleAlert, DatabaseBackup, Download, HardDrive, ShieldCheck } from '@lucide/vue'
import { computed, ref } from 'vue'
import { backupDownloadURL, backupKeys, createBackup, deleteBackup, getBackupOverview } from '../api/backups'

const queryClient = useQueryClient()
const confirmationOpen = ref(false)
const recoveryDetailsOpen = ref(false)
const error = ref('')
const pageCursors = ref([''])
const pageIndex = ref(0)
const currentCursor = computed(() => pageCursors.value[pageIndex.value] ?? '')
const deletionTarget = ref<BackupSummary | null>(null)
const deletionConfirmation = ref('')
const deletionError = ref('')
const terminalStatuses = new Set<BackupStatus>(['complete', 'failed'])
const overview = useQuery({
  queryKey: computed(() => backupKeys.overview(currentCursor.value)),
  queryFn: () => getBackupOverview(currentCursor.value),
  refetchInterval: (query) => {
    const status = query.state.data?.activeOperation?.status
    return status && !terminalStatuses.has(status) ? 1500 : 10_000
  },
})
const operation = computed(() => overview.data.value?.activeOperation)
const operationRunning = computed(() => {
  const status = operation.value?.status
  return status != null && !terminalStatuses.has(status)
})
const create = useMutation({
  mutationFn: createBackup,
  onMutate: () => {
    error.value = ''
  },
  onSuccess: async () => {
    confirmationOpen.value = false
    pageCursors.value = ['']
    pageIndex.value = 0
    await queryClient.invalidateQueries({ queryKey: backupKeys.all })
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not start the backup'
  },
})
const remove = useMutation({
  mutationFn: deleteBackup,
  onMutate: () => {
    deletionError.value = ''
  },
  onSuccess: async () => {
    if ((overview.data.value?.backups.length ?? 0) === 1 && pageIndex.value > 0) {
      pageCursors.value = pageCursors.value.slice(0, pageIndex.value)
      pageIndex.value -= 1
    }
    deletionTarget.value = null
    deletionConfirmation.value = ''
    await queryClient.invalidateQueries({ queryKey: backupKeys.all })
  },
  onError: (caught) => {
    deletionError.value = caught instanceof APIError ? caught.message : 'Could not delete the recovery point'
  },
})
const pageStart = computed(() =>
  overview.data.value?.totalBackups ? pageIndex.value * overview.data.value.pageSize + 1 : 0,
)
const pageEnd = computed(() =>
  Math.min(pageStart.value + (overview.data.value?.backups.length ?? 0) - 1, overview.data.value?.totalBackups ?? 0),
)

function nextPage() {
  const nextCursor = overview.data.value?.nextCursor
  if (!nextCursor) return
  pageCursors.value = [...pageCursors.value.slice(0, pageIndex.value + 1), nextCursor]
  pageIndex.value += 1
}

function previousPage() {
  if (pageIndex.value > 0) pageIndex.value -= 1
}

function openDeletion(backup: BackupSummary) {
  deletionTarget.value = backup
  deletionConfirmation.value = ''
  deletionError.value = ''
}

function closeDeletion() {
  if (remove.isPending.value) return
  deletionTarget.value = null
  deletionConfirmation.value = ''
  deletionError.value = ''
}

function confirmDeletion() {
  if (deletionTarget.value && deletionConfirmation.value === 'DELETE') {
    remove.mutate(deletionTarget.value.backupId)
  }
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes / 1024
  let unit = units[0]
  for (let index = 1; index < units.length && value >= 1024; index += 1) {
    value /= 1024
    unit = units[index]
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${unit}`
}

function statusTone(status?: BackupStatus) {
  if (status && terminalStatuses.has(status)) {
    return status === 'failed' ? 'danger' : 'success'
  }
  return 'warning'
}

function downloadBackup(backupId: string) {
  globalThis.location.assign(backupDownloadURL(backupId))
}
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Installation operations</p>
        <h1 class="page-title">Backup & recovery</h1>
        <p class="page-description">Create verified recovery points without checking out the Grom repository or granting Docker control to the application.</p>
      </div>
      <ActionButton
        :disabled="!overview.data.value?.available || operationRunning"
        @click="confirmationOpen = true"
      >
        <DatabaseBackup :size="17" />
        Create backup
      </ActionButton>
    </header>

    <div v-if="overview.data.value && !overview.data.value.available" class="operation-warning" role="alert">
      <CircleAlert :size="18" />
      <div>
        <strong>Integrated backup agent unavailable</strong>
        <p>The shipped deployment starts this internal agent automatically. Check its health before relying on recovery points.</p>
      </div>
    </div>

    <section class="overview-grid" aria-label="Backup overview">
      <Card class="overview-card">
        <div class="overview-icon"><ShieldCheck :size="18" /></div>
        <div>
          <p class="overview-label">Consistency</p>
          <p class="overview-value">Quiesced</p>
          <p class="overview-detail">New registry and management writes are drained before capture.</p>
        </div>
      </Card>
      <Card class="overview-card">
        <div class="overview-icon"><HardDrive :size="18" /></div>
        <div>
          <p class="overview-label">Local recovery points</p>
          <p class="overview-value">{{ overview.data.value?.totalBackups ?? '—' }}</p>
          <p class="overview-detail">Download and retain copies outside this host.</p>
        </div>
      </Card>
      <Card class="overview-card">
        <div class="overview-icon"><Archive :size="18" /></div>
        <div>
          <p class="overview-label">Current operation</p>
          <div class="mt-2">
            <Badge :tone="statusTone(operation?.status)">
              {{ operation?.status ?? 'idle' }}
            </Badge>
          </div>
          <p class="overview-detail">{{ operation?.message ?? 'No backup is currently running.' }}</p>
        </div>
      </Card>
    </section>

    <section class="backups-panel">
      <div class="panel-heading">
        <div>
          <h2>Recovery points</h2>
          <p>Every download is a complete, checksummed bundle containing metadata, signing material, and registry content.</p>
        </div>
      </div>
      <div v-if="overview.isLoading.value" class="backup-list">
        <div v-for="index in 3" :key="index" class="backup-row backup-row-loading" />
      </div>
      <div v-else-if="overview.isError.value" class="empty-state" role="alert">
        <CircleAlert class="mx-auto mb-3 text-destructive" :size="30" />
        <p class="font-medium text-foreground">Could not load recovery points</p>
        <p class="mt-1 text-sm">Check the Grom service and try again.</p>
      </div>
      <div v-else-if="!overview.data.value?.backups.length" class="empty-state">
        <DatabaseBackup class="mx-auto mb-3 text-accent" :size="30" />
        <p class="font-medium text-foreground">No recovery points yet</p>
        <p class="mt-1 text-sm">Create a backup, then download it to encrypted off-host storage.</p>
      </div>
      <div v-else class="backup-list">
        <article v-for="backup in overview.data.value.backups" :key="backup.backupId" class="backup-row">
          <div class="backup-identity">
            <div class="backup-icon"><Archive :size="17" /></div>
            <div>
              <p class="font-medium">{{ new Date(backup.createdAt).toLocaleString() }}</p>
              <p class="backup-id">{{ backup.backupId }}</p>
            </div>
          </div>
          <div>
            <p class="backup-meta-label">Protected data</p>
            <p class="backup-meta-value">{{ formatBytes(backup.totalBytes) }}</p>
          </div>
          <div>
            <p class="backup-meta-label">Grom version</p>
            <p class="backup-meta-value">{{ backup.gromVersion }}</p>
          </div>
          <div class="backup-actions">
            <Button variant="outline" size="sm" @click="downloadBackup(backup.backupId)">
              <Download :size="15" />
              Download
            </Button>
            <DeleteButton
              size="icon"
              :aria-label="`Delete backup ${backup.backupId}`"
              :disabled="operationRunning || remove.isPending.value"
              @click="openDeletion(backup)"
            />
          </div>
        </article>
      </div>
      <div v-if="overview.data.value?.totalBackups" class="pagination-bar">
        <p>{{ pageStart }}–{{ pageEnd }} of {{ overview.data.value.totalBackups }}</p>
        <div>
          <Button variant="outline" size="sm" :disabled="pageIndex === 0" @click="previousPage">
            <ChevronLeft :size="15" />
            Previous
          </Button>
          <span>Page {{ pageIndex + 1 }}</span>
          <Button variant="outline" size="sm" :disabled="!overview.data.value.nextCursor" @click="nextPage">
            Next
            <ChevronRight :size="15" />
          </Button>
        </div>
      </div>
    </section>

    <Card class="recovery-note">
      <div>
        <h2>Recovering a lost installation</h2>
        <p>The same Grom image includes a dedicated, loopback-only recovery UI. Review the setup before beginning a restore.</p>
      </div>
      <Button variant="outline" @click="recoveryDetailsOpen = true">View details</Button>
    </Card>

    <div v-if="recoveryDetailsOpen" class="modal-backdrop" @click.self="recoveryDetailsOpen = false">
      <section
        class="modal recovery-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="recovery-details-title"
      >
        <div class="recovery-modal-header">
          <div>
            <p class="eyebrow">Disaster recovery</p>
            <h2 id="recovery-details-title" class="text-lg font-semibold">Recovering a lost installation</h2>
            <p>The same Grom image includes a dedicated, loopback-only recovery UI. Keep a downloaded recovery bundle available before beginning.</p>
          </div>
          <Button variant="outline" size="sm" @click="recoveryDetailsOpen = false">Close</Button>
        </div>
        <div class="recovery-warning">
          <CircleAlert :size="17" />
          <span>Recovery accepts only empty replacement volumes. Never delete a healthy installation merely to test this procedure.</span>
        </div>
        <ol class="recovery-steps">
          <li>
            <strong>Download a recovery bundle before an incident.</strong>
            <span>In <strong>Backup &amp; recovery</strong>, find a completed recovery point and select <strong>Download</strong>. Store the resulting <code>.tar</code> file in encrypted off-host storage that remains accessible if this server is lost.</span>
          </li>
          <li>
            <strong>Obtain the deployment files.</strong>
            <span>Clone the official repository, then enter its directory. This provides the Compose definition; the recovery tooling itself is already inside the Grom image.</span>
            <code>git clone https://github.com/jfxdev/grom-registry.git<br />cd grom-registry</code>
          </li>
          <li>
            <strong>Stop the normal installation.</strong>
            <span>Grom and Distribution must remain stopped throughout recovery.</span>
          </li>
          <li>
            <strong>Start recovery mode from the deployment directory.</strong>
            <code>docker compose --env-file .env -f deploy/compose/docker-compose.yml --profile recovery up recovery</code>
          </li>
          <li>
            <strong>Open the local recovery UI.</strong>
            <span>Visit <code>http://127.0.0.1:8081</code>, or the loopback port configured by <code>GROM_RECOVERY_PORT</code>.</span>
          </li>
          <li>
            <strong>Upload and verify the bundle.</strong>
            <span>Select the downloaded <code>.tar</code>, choose the verified recovery point, type <code>RESTORE</code>, and wait for completion.</span>
          </li>
          <li>
            <strong>Return to normal mode.</strong>
            <span>Stop the recovery container, then start the normal deployment. Grom will invalidate restored web sessions during its first boot.</span>
          </li>
        </ol>
      </section>
    </div>

    <div v-if="confirmationOpen" class="modal-backdrop" @click.self="confirmationOpen = false">
      <form class="modal form-stack" @submit.prevent="create.mutate()">
        <div>
          <h2 class="text-lg font-semibold">Create recovery point?</h2>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">Grom will briefly pause registry access and reject management changes while active writes drain and the consistent snapshot is created. Read-only pages in this interface remain available.</p>
        </div>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="flex justify-end gap-2">
          <CancelButton @click="confirmationOpen = false" />
          <ActionButton type="submit" :disabled="create.isPending.value">Begin backup</ActionButton>
        </div>
      </form>
    </div>

    <div v-if="deletionTarget" class="modal-backdrop" @click.self="closeDeletion">
      <form class="modal form-stack" @submit.prevent="confirmDeletion">
        <div>
          <h2 class="text-lg font-semibold">Delete this recovery point?</h2>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">This permanently removes the local snapshot. Bundles already downloaded to another location are not affected.</p>
        </div>
        <div class="deletion-summary">
          <p>{{ new Date(deletionTarget.createdAt).toLocaleString() }}</p>
          <code>{{ deletionTarget.backupId }}</code>
          <span>{{ formatBytes(deletionTarget.totalBytes) }}</span>
        </div>
        <label class="confirmation-field">
          <span>Type <strong>DELETE</strong> to confirm</span>
          <input
            v-model="deletionConfirmation"
            aria-label="Type DELETE to confirm"
            autocomplete="off"
            placeholder="DELETE"
          />
        </label>
        <p v-if="deletionError" class="error-text">{{ deletionError }}</p>
        <div class="flex justify-end gap-2">
          <CancelButton :disabled="remove.isPending.value" @click="closeDeletion" />
          <DeleteButton
            type="submit"
            :disabled="deletionConfirmation !== 'DELETE' || remove.isPending.value"
          >
            {{ remove.isPending.value ? 'Deleting…' : 'Delete snapshot' }}
          </DeleteButton>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.operation-warning {
  display: flex;
  gap: .75rem;
  margin-bottom: 1rem;
  border: 1px solid color-mix(in srgb, var(--warning) 28%, transparent);
  border-radius: .75rem;
  background: color-mix(in srgb, var(--warning) 7%, transparent);
  padding: .9rem 1rem;
  color: var(--foreground);
}
.operation-warning p { margin-top: .2rem; color: var(--muted-foreground); font-size: .82rem; }
.overview-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .85rem; margin-bottom: 1rem; }
.overview-card { display: flex; min-height: 9.5rem; gap: 1rem; }
.overview-icon, .backup-icon {
  display: grid; flex: 0 0 auto; width: 2.35rem; height: 2.35rem; place-items: center;
  border: 1px solid rgba(145, 173, 36, .16); border-radius: .7rem;
  background: rgba(145, 173, 36, .08); color: #c9df6c;
}
.overview-label, .backup-meta-label { color: var(--muted-foreground); font-size: .72rem; font-weight: 600; }
.overview-value { margin-top: .3rem; font-size: 1.25rem; font-weight: 680; }
.overview-detail { margin-top: .65rem; color: var(--muted-foreground); font-size: .78rem; line-height: 1.45; }
.backups-panel { overflow: hidden; border: 1px solid var(--border); border-radius: .8rem; background: var(--card); }
.panel-heading { padding: 1.15rem 1.25rem; border-bottom: 1px solid var(--border); }
.panel-heading h2 { font-weight: 650; }
.panel-heading p { margin-top: .25rem; color: var(--muted-foreground); font-size: .8rem; }
.backup-list { display: grid; }
.backup-row {
  display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(7rem, .6fr) minmax(7rem, .6fr) auto;
  align-items: center; gap: 1rem; min-height: 5rem; padding: .8rem 1.25rem; border-bottom: 1px solid var(--border);
}
.backup-row:last-child { border-bottom: 0; }
.backup-row-loading { margin: .65rem 1rem; min-height: 3.5rem; border: 0; border-radius: .55rem; background: var(--surface-raised); }
.backup-identity { display: flex; align-items: center; gap: .8rem; min-width: 0; }
.backup-id { margin-top: .15rem; overflow: hidden; color: var(--muted-foreground); font-family: monospace; font-size: .7rem; text-overflow: ellipsis; }
.backup-meta-value { margin-top: .18rem; font-size: .82rem; }
.backup-actions { display: flex; align-items: center; gap: .45rem; }
.pagination-bar {
  display: flex; align-items: center; justify-content: space-between; gap: 1rem;
  border-top: 1px solid var(--border); padding: .8rem 1.25rem;
  color: var(--muted-foreground); font-size: .78rem;
}
.pagination-bar > div { display: flex; align-items: center; gap: .65rem; }
.pagination-bar span { min-width: 3.8rem; text-align: center; }
.deletion-summary {
  display: grid; gap: .25rem; border: 1px solid var(--border); border-radius: .65rem;
  background: var(--surface-raised); padding: .8rem .9rem; font-size: .82rem;
}
.deletion-summary code { overflow: hidden; color: var(--muted-foreground); font-size: .72rem; text-overflow: ellipsis; }
.deletion-summary span { color: var(--muted-foreground); font-size: .75rem; }
.confirmation-field { display: grid; gap: .45rem; color: var(--muted-foreground); font-size: .8rem; }
.confirmation-field input {
  min-height: 2.65rem; border: 1px solid var(--border); border-radius: .55rem;
  background: var(--surface-raised); color: var(--foreground); padding: .55rem .7rem;
}
.recovery-note {
  display: flex; align-items: center; justify-content: space-between; gap: 1rem;
  margin-top: 1rem; border-color: rgba(169, 120, 50, .3);
}
.recovery-note h2 { font-weight: 650; }
.recovery-note > div > p { margin-top: .45rem; color: var(--muted-foreground); font-size: .83rem; line-height: 1.6; }
.recovery-modal {
  width: min(100%, 46rem); max-height: calc(100vh - 2rem); overflow-y: auto;
}
.recovery-modal-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.recovery-modal-header > div > p:last-child {
  margin-top: .45rem; color: var(--muted-foreground); font-size: .83rem; line-height: 1.6;
}
.recovery-modal code { color: var(--foreground); font-family: monospace; }
.recovery-warning {
  display: flex; align-items: flex-start; gap: .55rem; margin-top: 1rem;
  border: 1px solid color-mix(in srgb, var(--warning) 28%, transparent);
  border-radius: .65rem; background: color-mix(in srgb, var(--warning) 6%, transparent);
  padding: .75rem .85rem; color: var(--muted-foreground); font-size: .8rem; line-height: 1.5;
}
.recovery-warning svg { flex: 0 0 auto; margin-top: .1rem; color: var(--warning); }
.recovery-steps { display: grid; gap: .85rem; margin: 1rem 0 0; padding: 0; list-style: none; counter-reset: recovery-step; }
.recovery-steps li {
  display: grid; grid-template-columns: 1.8rem minmax(0, 1fr); gap: .2rem .65rem;
  color: var(--muted-foreground); font-size: .8rem; line-height: 1.55; counter-increment: recovery-step;
}
.recovery-steps li::before {
  content: counter(recovery-step); grid-row: 1 / span 3; display: grid; width: 1.65rem; height: 1.65rem;
  place-items: center; border: 1px solid rgba(145, 173, 36, .22); border-radius: 999px;
  background: rgba(145, 173, 36, .08); color: #c9df6c; font-size: .7rem; font-weight: 700;
}
.recovery-steps strong { color: var(--foreground); font-size: .82rem; }
.recovery-steps code {
  overflow-x: auto; margin-top: .25rem; border: 1px solid var(--border); border-radius: .45rem;
  background: var(--surface-raised); padding: .55rem .65rem; font-size: .72rem; white-space: nowrap;
}
@media (max-width: 900px) {
  .overview-grid { grid-template-columns: 1fr; }
  .backup-row { grid-template-columns: 1fr auto; }
  .backup-row > div:nth-child(2), .backup-row > div:nth-child(3) { display: none; }
  .backup-actions { align-items: stretch; flex-direction: column; }
  .pagination-bar { align-items: flex-start; flex-direction: column; }
  .recovery-note, .recovery-modal-header { align-items: stretch; flex-direction: column; }
}
</style>
