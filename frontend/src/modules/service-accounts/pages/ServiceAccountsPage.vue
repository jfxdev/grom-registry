<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { APIError } from '@/shared/api/client'
import type { ServiceAccount } from '@/shared/api/models'
import { Badge } from '@/shared/components/ui/badge'
import { ActionButton, Button, CancelButton, DeleteButton } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Bot, ChevronDown, KeyRound, Plus, Search, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import ServiceAccountKeysPanel from '../components/ServiceAccountKeysPanel.vue'
import {
  createServiceAccount,
  disableServiceAccount,
  listServiceAccounts,
  serviceAccountKeys,
} from '../api/serviceAccounts'

const queryClient = useQueryClient()
const session = useSessionStore()
const accounts = useQuery({ queryKey: serviceAccountKeys.list(true), queryFn: () => listServiceAccounts(true) })
const modalOpen = ref(false)
const selectedAccountId = ref<string | null>(null)
const name = ref('')
const username = ref('')
const description = ref('')
const error = ref('')
const searchQuery = ref('')
const statusFilter = ref<'active' | 'disabled' | 'all'>('active')
const disableTarget = ref<ServiceAccount | null>(null)
const disableConfirmation = ref('')
const disableError = ref('')
const statusAccounts = computed(() => (accounts.data.value ?? []).filter((account) => {
  if (statusFilter.value === 'all') return true
  return statusFilter.value === 'disabled' ? Boolean(account.disabledAt) : !account.disabledAt
}))
const filteredAccounts = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  if (!query) return statusAccounts.value

  return statusAccounts.value.filter((account) =>
    account.name.toLocaleLowerCase().includes(query)
      || account.username.toLocaleLowerCase().includes(query)
      || account.description.toLocaleLowerCase().includes(query),
  )
})

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

const disable = useMutation({
  mutationFn: disableServiceAccount,
  onMutate: () => {
    disableError.value = ''
  },
  onSuccess: async () => {
    disableTarget.value = null
    disableConfirmation.value = ''
    await queryClient.invalidateQueries({ queryKey: serviceAccountKeys.all })
  },
  onError: (caught) => {
    disableError.value = caught instanceof APIError ? caught.message : 'Could not disable the service account'
  },
})

function openDisable(account: ServiceAccount) {
  disableTarget.value = account
  disableConfirmation.value = ''
  disableError.value = ''
}

function closeDisable() {
  if (disable.isPending.value) return
  disableTarget.value = null
  disableConfirmation.value = ''
  disableError.value = ''
}

function confirmDisable() {
  if (disableTarget.value && disableConfirmation.value === disableTarget.value.name) {
    disable.mutate(disableTarget.value.id)
  }
}
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

    <div class="account-list">
      <div class="list-toolbar">
        <div>
          <h2>All service accounts</h2>
          <p>Automation identities used by registry clients.</p>
        </div>
        <div class="list-toolbar-actions">
          <select v-model="statusFilter" class="field-control status-filter" aria-label="Filter service accounts by status">
            <option value="active">Active</option>
            <option value="disabled">Disabled</option>
            <option value="all">All statuses</option>
          </select>
          <div class="list-search">
            <Search :size="16" aria-hidden="true" />
            <Input
              v-model="searchQuery"
              type="search"
              placeholder="Search service accounts"
              aria-label="Search service accounts"
            />
          </div>
          <span v-if="!accounts.isLoading.value" class="list-count">
            {{ searchQuery.trim() ? `${filteredAccounts.length} of ${statusAccounts.length}` : `${statusAccounts.length} total` }}
          </span>
        </div>
      </div>

      <div v-if="accounts.isLoading.value" class="empty-state list-empty-state" role="status">
        <p class="text-sm text-muted-foreground">Loading service accounts…</p>
      </div>
      <div v-else-if="!statusAccounts.length" class="empty-state list-empty-state">
        <div>
          <Bot class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">
            {{ statusFilter === 'disabled' ? 'No disabled service accounts' : statusFilter === 'active' ? 'No active service accounts' : 'No service accounts' }}
          </p>
        </div>
      </div>
      <div v-else-if="!filteredAccounts.length" class="empty-state list-empty-state">
        <div>
          <Search class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No matching service accounts</p>
          <p class="mt-1 text-sm">Try a different name, username or description.</p>
        </div>
      </div>
      <template v-else>
        <div v-for="account in filteredAccounts" :key="account.id" class="account-item">
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
              <Badge :tone="account.disabledAt ? 'neutral' : 'success'">{{ account.disabledAt ? 'Disabled' : 'Active' }}</Badge>
              <Button
                v-if="session.user?.systemAdmin && !account.disabledAt"
                variant="outline"
                size="sm"
                :aria-expanded="selectedAccountId === account.id"
                @click="selectedAccountId = selectedAccountId === account.id ? null : account.id"
              >
                <KeyRound :size="15" />
                Keys
                <ChevronDown class="transition-transform" :class="{ 'rotate-180': selectedAccountId === account.id }" :size="14" />
              </Button>
              <DeleteButton
                v-if="session.user?.systemAdmin && !account.disabledAt"
                size="icon"
                :aria-label="`Disable service account ${account.name}`"
                @click="openDisable(account)"
              />
            </div>
          </div>
          <ServiceAccountKeysPanel v-if="!account.disabledAt && selectedAccountId === account.id" :account="account" />
        </div>
      </template>
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

    <div v-if="disableTarget && session.user?.systemAdmin" class="modal-backdrop" @click.self="closeDisable">
      <form class="modal form-stack" aria-labelledby="disable-service-account-title" @submit.prevent="confirmDisable">
        <div>
          <p class="eyebrow">Destructive action</p>
          <h2 id="disable-service-account-title" class="text-lg font-semibold">Disable service account</h2>
          <p class="mt-2 text-sm leading-6 text-muted-foreground">
            Disabling <strong>{{ disableTarget.name }}</strong> blocks all of its access keys.
            Automations using those keys will lose access to the registry.
          </p>
        </div>
        <label class="field-label">
          Type <strong>{{ disableTarget.name }}</strong> to confirm
          <Input
            v-model="disableConfirmation"
            aria-label="Service account name confirmation"
            autocomplete="off"
            :placeholder="disableTarget.name"
          />
        </label>
        <p v-if="disableError" class="error-text" role="alert">{{ disableError }}</p>
        <div class="flex justify-end gap-2">
          <CancelButton :disabled="disable.isPending.value" @click="closeDisable" />
          <DeleteButton
            type="submit"
            :disabled="disableConfirmation !== disableTarget.name || disable.isPending.value"
          >
            {{ disable.isPending.value ? 'Disabling…' : 'Disable service account' }}
          </DeleteButton>
        </div>
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

.list-toolbar {
  display: flex;
  min-height: 4.65rem;
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

.list-toolbar-actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: 0.65rem;
}

.status-filter {
  width: auto;
  min-width: 7.5rem;
  min-height: 2.35rem;
  padding: 0 0.7rem;
  font-size: 0.75rem;
}

.list-search {
  position: relative;
  width: min(18rem, 34vw);
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

.list-count {
  border: 1px solid var(--border);
  border-radius: 0.45rem;
  padding: 0.3rem 0.5rem;
  color: var(--muted-foreground);
  font-size: 0.7rem;
}

.list-empty-state {
  min-height: 16rem;
  border: 0;
  border-radius: 0;
}

.account-item {
  border-top: 1px solid var(--border);
}

.account-item:first-child {
  border-top: 0;
}

.list-toolbar + .account-item {
  border-top: 0;
}

.account-item .data-row {
  border-top: 0;
}

@media (max-width: 600px) {
  .list-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .list-toolbar-actions,
  .list-search {
    width: 100%;
  }

  .list-toolbar-actions {
    flex-wrap: wrap;
  }

  .list-search {
    flex: 1;
    min-width: 12rem;
  }
}
</style>
