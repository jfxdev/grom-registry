<script setup lang="ts">
import { getInstallationStatus, runGarbageCollection } from '@/modules/settings/api/settings'
import { Button } from '@/shared/components/ui/button'
import { Badge } from '@/shared/components/ui/badge'
import { Card } from '@/shared/components/ui/card'
import { useMutation, useQuery } from '@tanstack/vue-query'
import { CircleAlert, Database, HardDrive, RefreshCw, Server, Trash2 } from '@lucide/vue'

const status = useQuery({ queryKey: ['installation-status'], queryFn: getInstallationStatus, refetchInterval: 15_000 })
const garbageCollection = useMutation({ mutationFn: runGarbageCollection, onSuccess: () => status.refetch() })
const formatBytes = (bytes: number) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 1, notation: 'compact', style: 'unit', unit: 'byte', unitDisplay: 'short' }).format(bytes)
</script>

<template>
  <main class="page-shell">
    <header class="page-header"><div><p class="eyebrow">Installation</p><h1>Settings</h1><p class="page-description">Details about the current installation.</p></div><button class="icon-button" aria-label="Refresh status" @click="status.refetch()"><RefreshCw :size="17" /></button></header>
    <p v-if="status.isError.value" class="error-banner"><CircleAlert :size="17" /> Could not load installation status.</p>
    <p v-else-if="status.isPending.value" class="status-pending">Loading installation status…</p>
    <section v-else-if="status.data.value" class="settings-grid">
      <Card class="status-card"><Database :size="22" class="text-accent" /><div><p class="status-label">Application database</p><p class="status-value">{{ status.data.value.database === 'postgres' ? 'PostgreSQL' : 'SQLite' }}</p><Badge tone="success">Configured</Badge></div></Card>
      <Card class="status-card"><Server :size="22" class="text-accent" /><div><p class="status-label">Distribution</p><p class="status-value">OCI registry engine</p><Badge :tone="status.data.value.distribution === 'available' ? 'success' : 'danger'">{{ status.data.value.distribution === 'available' ? 'Available' : 'Unavailable' }}</Badge></div></Card>
      <Card class="status-card"><HardDrive :size="22" class="text-accent" /><div><p class="status-label">Registry storage</p><p class="status-value">{{ status.data.value.storage ? formatBytes(status.data.value.storage.usedBytes) : 'Unavailable' }}</p><p class="text-sm text-muted-foreground">Physical files currently used by Distribution.</p></div></Card>
      <Card class="status-card"><Trash2 :size="22" class="text-accent" /><div><p class="status-label">Garbage collection</p><p class="status-value">Reclaim deleted blobs</p><p class="mb-3 text-sm text-muted-foreground">Writes pause briefly while Distribution collects unreferenced data.</p><Button size="sm" :loading="garbageCollection.isPending.value" :disabled="status.data.value.distribution !== 'available' || status.data.value.storage === null" @click="garbageCollection.mutate()">Run garbage collection</Button><p v-if="garbageCollection.data.value" class="mt-2 text-sm text-muted-foreground">Reclaimed {{ formatBytes(garbageCollection.data.value.reclaimedBytes) }}.</p><p v-if="garbageCollection.isError.value" class="mt-2 text-sm text-destructive">Garbage collection could not complete.</p></div></Card>
    </section>
  </main>
</template>

<style scoped>
.settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.status-card{display:flex;gap:1rem;align-items:flex-start;padding:1.35rem}.status-label{margin:0;color:var(--muted-foreground);font-size:.875rem}.status-value{margin:.25rem 0 .7rem;font-size:1.15rem;font-weight:700}.icon-button{border:1px solid var(--border);border-radius:.5rem;padding:.55rem;background:var(--card);box-shadow:0 2px 0 var(--border),0 4px 10px color-mix(in srgb,var(--foreground) 12%,transparent);cursor:pointer;transition:transform .15s ease,box-shadow .15s ease}.icon-button:active{transform:translateY(2px);box-shadow:0 0 0 var(--border),0 1px 4px color-mix(in srgb,var(--foreground) 10%,transparent)}.error-banner,.status-pending{display:flex;gap:.5rem;color:var(--destructive)}.status-pending{color:var(--muted-foreground)}@media(max-width:640px){.settings-grid{grid-template-columns:1fr}}
</style>
