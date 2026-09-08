<script setup lang="ts">
import { getInstallationDiagnostics, runGarbageCollection } from '@/modules/settings/api/settings'
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { useMutation, useQuery } from '@tanstack/vue-query'
import { CircleAlert, Database, DatabaseBackup, HardDrive, KeyRound, RefreshCw, Server, ShieldCheck, Trash2 } from '@lucide/vue'

const diagnostics = useQuery({
  queryKey: ['installation-diagnostics'],
  queryFn: getInstallationDiagnostics,
  refetchInterval: 15_000,
})
const garbageCollection = useMutation({
  mutationFn: runGarbageCollection,
  onSuccess: () => diagnostics.refetch(),
})

const formatBytes = (bytes: number) => new Intl.NumberFormat(undefined, {
  maximumFractionDigits: 1,
  notation: 'compact',
  style: 'unit',
  unit: 'byte',
  unitDisplay: 'short',
}).format(bytes)

const formatDate = (value: string | null) => value ? new Date(value).toLocaleString() : 'No recovery point yet'
const formatProfile = (value: string) => value.charAt(0).toUpperCase() + value.slice(1)
const availabilityTone = (status: string) => status === 'available' || status === 'loaded' || status === 'current' ? 'success' : status === 'pending' ? 'warning' : 'danger'
const availabilityLabel = (status: string) => status === 'current' ? 'Current' : status === 'pending' ? 'Pending' : status === 'loaded' ? 'Loaded' : status === 'available' ? 'Available' : 'Unavailable'
</script>

<template>
  <main class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Installation</p>
        <h1>Installation</h1>
        <p class="page-description">Live diagnostics for this Grom installation. Each service is checked independently.</p>
        <p v-if="diagnostics.data.value" class="checked-at">Last checked {{ new Date(diagnostics.data.value.checkedAt).toLocaleString() }}<span v-if="diagnostics.isFetching.value"> · Refreshing…</span></p>
      </div>
      <button class="icon-button" aria-label="Refresh diagnostics" :disabled="diagnostics.isFetching.value" @click="diagnostics.refetch()"><RefreshCw :size="17" /></button>
    </header>

    <p v-if="diagnostics.isError.value && !diagnostics.data.value" class="error-banner" role="alert"><CircleAlert :size="17" /> Could not load installation diagnostics.</p>
    <p v-else-if="diagnostics.isPending.value" class="status-pending">Loading installation diagnostics…</p>
    <template v-else-if="diagnostics.data.value">
      <p v-if="diagnostics.isError.value" class="error-banner" role="alert"><CircleAlert :size="17" /> Could not refresh diagnostics. Showing the most recent result.</p>

      <section class="diagnostics-section" aria-labelledby="diagnostics-heading">
        <div class="section-heading"><div><p class="eyebrow">Read-only</p><h2 id="diagnostics-heading">Diagnostics</h2></div></div>
        <div class="diagnostics-grid">
          <Card class="diagnostic-card">
            <Database :size="22" class="text-accent" />
            <div><p class="status-label">Database</p><p class="status-value">{{ diagnostics.data.value.database.kind === 'postgres' ? 'PostgreSQL' : 'SQLite' }}</p><Badge :tone="availabilityTone(diagnostics.data.value.database.status)">{{ availabilityLabel(diagnostics.data.value.database.status) }}</Badge></div>
          </Card>
          <Card class="diagnostic-card">
            <Database :size="22" class="text-accent" />
            <div><p class="status-label">Schema migration</p><p class="status-value">{{ diagnostics.data.value.migration.appliedVersion ?? 'No migration applied' }}</p><Badge :tone="availabilityTone(diagnostics.data.value.migration.status)">{{ availabilityLabel(diagnostics.data.value.migration.status) }}</Badge><p v-if="diagnostics.data.value.migration.appliedAt" class="diagnostic-detail">Applied {{ new Date(diagnostics.data.value.migration.appliedAt).toLocaleString() }}</p></div>
          </Card>
          <Card class="diagnostic-card">
            <Server :size="22" class="text-accent" />
            <div><p class="status-label">Distribution</p><p class="status-value">{{ diagnostics.data.value.distribution.apiVersion ?? 'Not reported' }}</p><Badge :tone="availabilityTone(diagnostics.data.value.distribution.status)">{{ availabilityLabel(diagnostics.data.value.distribution.status) }}</Badge><p class="diagnostic-detail">Reported Registry API version.</p></div>
          </Card>
          <Card class="diagnostic-card">
            <KeyRound :size="22" class="text-accent" />
            <div><p class="status-label">Token signing</p><p class="status-value">{{ diagnostics.data.value.signing.algorithm ?? 'Not reported' }}</p><Badge :tone="availabilityTone(diagnostics.data.value.signing.status)">{{ availabilityLabel(diagnostics.data.value.signing.status) }}</Badge><p v-if="diagnostics.data.value.signing.keyId" class="diagnostic-detail">Key ID: {{ diagnostics.data.value.signing.keyId }}</p></div>
          </Card>
          <Card class="diagnostic-card">
            <ShieldCheck :size="22" class="text-accent" />
            <div><p class="status-label">Deployment profile</p><p class="status-value">{{ formatProfile(diagnostics.data.value.deployment.profile) }}</p><Badge :tone="diagnostics.data.value.deployment.insecureHttp ? 'warning' : 'success'">{{ diagnostics.data.value.deployment.insecureHttp ? 'Insecure HTTP' : 'Secure posture' }}</Badge><p v-if="diagnostics.data.value.deployment.insecureHttp" class="diagnostic-detail">Use this permissive profile only on a trusted private network.</p></div>
          </Card>
          <Card class="diagnostic-card">
            <HardDrive :size="22" class="text-accent" />
            <div><p class="status-label">Registry storage</p><p class="status-value">{{ diagnostics.data.value.storage.usedBytes === null ? 'Unavailable' : formatBytes(diagnostics.data.value.storage.usedBytes) }}</p><Badge :tone="availabilityTone(diagnostics.data.value.storage.status)">{{ availabilityLabel(diagnostics.data.value.storage.status) }}</Badge><p class="diagnostic-detail">Physical files currently used by Distribution.</p></div>
          </Card>
          <Card class="diagnostic-card">
            <DatabaseBackup :size="22" class="text-accent" />
            <div><p class="status-label">Last backup</p><p class="status-value">{{ formatDate(diagnostics.data.value.backup.lastBackupAt) }}</p><Badge :tone="availabilityTone(diagnostics.data.value.backup.status)">{{ availabilityLabel(diagnostics.data.value.backup.status) }}</Badge><p v-if="diagnostics.data.value.backup.status === 'available' && !diagnostics.data.value.backup.lastBackupAt" class="diagnostic-detail">Create and download a recovery point for off-host retention.</p></div>
          </Card>
        </div>
      </section>

      <section class="diagnostics-section maintenance-section" aria-labelledby="maintenance-heading">
        <div class="section-heading"><div><p class="eyebrow">Administrative action</p><h2 id="maintenance-heading">Maintenance</h2></div></div>
        <Card class="maintenance-card">
          <Trash2 :size="22" class="text-accent" />
          <div><p class="status-label">Garbage collection</p><p class="status-value">Reclaim deleted blobs</p><p class="mb-3 text-sm text-muted-foreground">Writes pause briefly while Distribution collects unreferenced data.</p><Button size="sm" :loading="garbageCollection.isPending.value" :disabled="diagnostics.data.value.distribution.status !== 'available' || diagnostics.data.value.storage.status !== 'available'" @click="garbageCollection.mutate()">Run garbage collection</Button><p v-if="garbageCollection.data.value" class="mt-2 text-sm text-muted-foreground">Reclaimed {{ formatBytes(garbageCollection.data.value.reclaimedBytes) }}.</p><p v-if="garbageCollection.isError.value" class="mt-2 text-sm text-destructive">Garbage collection could not complete.</p></div>
        </Card>
      </section>
    </template>
  </main>
</template>

<style scoped>
.checked-at,.diagnostic-detail{margin:.55rem 0 0;color:var(--muted-foreground);font-size:.8rem}.diagnostics-section{margin-top:1.5rem}.section-heading{margin-bottom:.8rem}.section-heading h2{margin:.1rem 0 0;font-size:1.15rem}.diagnostics-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.diagnostic-card,.maintenance-card{display:flex;gap:1rem;align-items:flex-start;padding:1.35rem}.status-label{margin:0;color:var(--muted-foreground);font-size:.875rem}.status-value{margin:.25rem 0 .7rem;font-size:1.15rem;font-weight:700}.maintenance-section{padding-top:1rem;border-top:1px solid var(--border)}.icon-button{border:1px solid var(--border);border-radius:.5rem;padding:.55rem;background:var(--card);box-shadow:0 2px 0 var(--border),0 4px 10px color-mix(in srgb,var(--foreground) 12%,transparent);cursor:pointer;transition:transform .15s ease,box-shadow .15s ease}.icon-button:disabled{cursor:wait;opacity:.7}.icon-button:active:not(:disabled){transform:translateY(2px);box-shadow:0 0 0 var(--border),0 1px 4px color-mix(in srgb,var(--foreground) 10%,transparent)}.error-banner,.status-pending{display:flex;gap:.5rem;color:var(--destructive)}.status-pending{color:var(--muted-foreground)}@media(max-width:640px){.diagnostics-grid{grid-template-columns:1fr}}
</style>
