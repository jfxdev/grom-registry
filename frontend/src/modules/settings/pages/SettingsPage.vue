<script setup lang="ts">
import { getInstallationStatus } from '@/modules/settings/api/settings'
import { Badge } from '@/shared/components/ui/badge'
import { Card } from '@/shared/components/ui/card'
import { useQuery } from '@tanstack/vue-query'
import { CircleAlert, Database, RefreshCw, Server } from '@lucide/vue'

const status = useQuery({ queryKey: ['installation-status'], queryFn: getInstallationStatus, refetchInterval: 15_000 })
</script>

<template>
  <main class="page-shell">
    <header class="page-header"><div><p class="eyebrow">Installation</p><h1>Settings</h1><p class="page-description">Details about the current installation.</p></div><button class="icon-button" aria-label="Refresh status" @click="status.refetch()"><RefreshCw :size="17" /></button></header>
    <p v-if="status.isError.value" class="error-banner"><CircleAlert :size="17" /> Could not load installation status.</p>
    <section v-else class="settings-grid">
      <Card class="status-card"><Database :size="22" class="text-accent" /><div><p class="status-label">Application database</p><p class="status-value">{{ status.data.value?.database === 'postgres' ? 'PostgreSQL' : 'SQLite' }}</p><Badge tone="success">Configured</Badge></div></Card>
      <Card class="status-card"><Server :size="22" class="text-accent" /><div><p class="status-label">Distribution</p><p class="status-value">OCI registry engine</p><Badge :tone="status.data.value?.distribution === 'available' ? 'success' : 'danger'">{{ status.data.value?.distribution === 'available' ? 'Available' : 'Unavailable' }}</Badge></div></Card>
    </section>
  </main>
</template>

<style scoped>
.settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.status-card{display:flex;gap:1rem;align-items:flex-start;padding:1.35rem}.status-label{margin:0;color:var(--muted-foreground);font-size:.875rem}.status-value{margin:.25rem 0 .7rem;font-size:1.15rem;font-weight:700}.icon-button{border:1px solid var(--border);border-radius:.5rem;padding:.55rem;background:var(--card);cursor:pointer}.error-banner{display:flex;gap:.5rem;color:var(--destructive)}@media(max-width:640px){.settings-grid{grid-template-columns:1fr}}
</style>
