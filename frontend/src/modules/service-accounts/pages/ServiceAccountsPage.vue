<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { APIError } from '@/shared/api/client'
import { Badge } from '@/shared/components/ui/badge'
import { ActionButton, Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Bot, ChevronDown, KeyRound, Plus, Trash2, X } from '@lucide/vue'
import { ref } from 'vue'
import ServiceAccountKeysPanel from '../components/ServiceAccountKeysPanel.vue'
import {
  createServiceAccount,
  deleteServiceAccount,
  listServiceAccounts,
  serviceAccountKeys,
} from '../api/serviceAccounts'

const queryClient = useQueryClient()
const session = useSessionStore()
const accounts = useQuery({ queryKey: serviceAccountKeys.all, queryFn: listServiceAccounts })
const modalOpen = ref(false)
const selectedAccountId = ref<string | null>(null)
const name = ref('')
const username = ref('')
const description = ref('')
const error = ref('')

const create = useMutation({
  mutationFn: createServiceAccount,
  onSuccess: async (account) => {
    await queryClient.invalidateQueries({ queryKey: serviceAccountKeys.all })
    selectedAccountId.value = account.id
    modalOpen.value = false
    name.value = ''
    username.value = ''
    description.value = ''
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create service account'
  },
})

const remove = useMutation({
  mutationFn: deleteServiceAccount,
  onSuccess: () => queryClient.invalidateQueries({ queryKey: serviceAccountKeys.all }),
})
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Automation identities</p>
        <h1 class="page-title">Service accounts</h1>
        <p class="page-description">Create stable identities for CI and deployment systems, then assign them to projects.</p>
      </div>
      <ActionButton v-if="session.user?.systemAdmin" @click="modalOpen = true"><Plus :size="17" /> New account</ActionButton>
    </header>

    <div v-if="!accounts.data.value?.length && !accounts.isLoading.value" class="empty-state">
      <div><Bot class="mx-auto mb-3 text-accent" :size="28" /><p class="font-medium text-foreground">No service accounts</p></div>
    </div>
    <div v-else class="account-list">
      <div v-for="account in accounts.data.value" :key="account.id" class="account-item">
        <div class="data-row">
          <div class="flex items-center gap-3">
            <div class="avatar"><Bot :size="16" /></div>
            <div>
              <p class="text-sm font-semibold">{{ account.name }}</p>
              <p class="mt-0.5 font-mono text-xs text-muted-foreground">{{ account.username }}</p>
            </div>
          </div>
          <p class="truncate text-sm text-muted-foreground">{{ account.description || 'No description' }}</p>
          <div class="flex items-center gap-2">
            <Badge tone="success">Active</Badge>
            <Button
              v-if="session.user?.systemAdmin"
              variant="outline"
              size="sm"
              :aria-expanded="selectedAccountId === account.id"
              @click="selectedAccountId = selectedAccountId === account.id ? null : account.id"
            >
              <KeyRound :size="15" />
              Keys
              <ChevronDown class="transition-transform" :class="{ 'rotate-180': selectedAccountId === account.id }" :size="14" />
            </Button>
            <Button
              v-if="session.user?.systemAdmin"
              variant="ghost"
              size="icon"
              aria-label="Disable service account"
              @click="remove.mutate(account.id)"
            >
              <Trash2 :size="15" />
            </Button>
          </div>
        </div>
        <ServiceAccountKeysPanel v-if="selectedAccountId === account.id" :account="account" />
      </div>
    </div>

    <div v-if="modalOpen" class="modal-backdrop" @click.self="modalOpen = false">
      <form class="modal form-stack" @submit.prevent="create.mutate({ name, username, description })">
        <div class="flex items-start justify-between">
          <div><h2 class="text-lg font-semibold">New service account</h2><p class="mt-1 text-sm text-muted-foreground">Use a lowercase username for Docker login.</p></div>
          <Button variant="ghost" size="icon" @click="modalOpen = false"><X :size="18" /></Button>
        </div>
        <label class="field-label">Display name<Input v-model="name" required /></label>
        <label class="field-label">Username<Input v-model="username" required placeholder="ci-payments" /></label>
        <label class="field-label">Description<Input v-model="description" placeholder="Production deployment pipeline" /></label>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" @click="modalOpen = false">Cancel</Button><Button type="submit">Create</Button></div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.account-list {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 0.9rem;
  background: color-mix(in srgb, var(--surface) 88%, transparent);
  box-shadow: 0 14px 40px rgba(0, 0, 0, 0.12);
}

.account-item {
  border-top: 1px solid var(--border);
}

.account-item:first-child {
  border-top: 0;
}

.account-item .data-row {
  border-top: 0;
}
</style>
