<script setup lang="ts">
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { useQuery } from '@tanstack/vue-query'
import { Cable, LockKeyhole } from '@lucide/vue'
import { integrationKeys, listIntegrations } from '../api/integrations'

const integrations = useQuery({ queryKey: integrationKeys.all, queryFn: listIntegrations })
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Extension points</p>
        <h1 class="page-title">Integrations</h1>
        <p class="page-description">Security scanners and event delivery are planned here. Nothing runs or receives secrets until a provider is explicitly implemented.</p>
      </div>
    </header>

    <div class="roadmap-note"><LockKeyhole :size="17" /><span>Roadmap preview — all providers are read-only and disabled.</span></div>
    <div class="grid-cards mt-5">
      <Card v-for="integration in integrations.data.value" :key="integration.key" class="flex min-h-56 flex-col justify-between">
        <div>
          <div class="flex items-start justify-between">
            <div class="integration-icon"><Cable :size="18" /></div>
            <Badge tone="warning">{{ integration.status }}</Badge>
          </div>
          <h2 class="mt-6 text-lg font-semibold">{{ integration.name }}</h2>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">
            {{ integration.capabilities.map((capability) => capability.replace(/-/g, ' ')).join(' · ') }}
          </p>
        </div>
        <Button variant="outline" disabled class="mt-6 w-full">Coming soon</Button>
      </Card>
    </div>
  </div>
</template>

<style scoped>
.roadmap-note {
  display: flex;
  align-items: center;
  gap: .6rem;
  border: 1px solid rgba(145, 173, 36, 0.16);
  border-radius: .75rem;
  background: rgba(145, 173, 36, 0.055);
  padding: .75rem 1rem;
  color: #c4bdb2;
  font-size: .8rem;
}
.integration-icon {
  display: grid;
  width: 2.4rem;
  height: 2.4rem;
  place-items: center;
  border-radius: .7rem;
  border: 1px solid rgba(145, 173, 36, 0.12);
  background: rgba(145, 173, 36, 0.08);
  color: #c9df6c;
}
</style>
