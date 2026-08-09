<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import type { ServiceAccount } from '@/shared/api/models'
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { PaginationControls } from '@/shared/components/ui/pagination'
import { writeClipboardText } from '@/shared/lib/clipboard'
import { pageItems, useCursorPagination } from '@/shared/lib/pagination'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Check, Copy, KeyRound, Plus, ShieldAlert, Trash2, X } from '@lucide/vue'
import { computed, onUnmounted, ref } from 'vue'
import {
  createServiceAccountToken,
  listServiceAccountTokens,
  revokeServiceAccountToken,
  serviceAccountKeys,
} from '../api/serviceAccounts'

const props = defineProps<{ account: ServiceAccount }>()
const queryClient = useQueryClient()
const pagination = useCursorPagination()
const keys = useQuery({
	queryKey: computed(() => [...serviceAccountKeys.tokens(props.account.id), pagination.cursor.value]),
  queryFn: () => listServiceAccountTokens(props.account.id, pagination.cursor.value),
})
const formOpen = ref(false)
const name = ref('')
const expiresAt = ref('')
const error = ref('')
const revealedSecret = ref('')
const copied = ref(false)
const copyError = ref('')
const currentTime = ref(Date.now())
const currentTimeTimer = window.setInterval(() => { currentTime.value = Date.now() }, 60_000)

onUnmounted(() => window.clearInterval(currentTimeTimer))

const create = useMutation({
  mutationFn: () => createServiceAccountToken(props.account.id, {
    name: name.value,
    expiresAt: expiresAt.value ? new Date(expiresAt.value).toISOString() : null,
  }),
  onSuccess: async (created) => {
    revealedSecret.value = created.secret
    name.value = ''
    expiresAt.value = ''
    formOpen.value = false
    error.value = ''
    await queryClient.invalidateQueries({ queryKey: serviceAccountKeys.tokens(props.account.id) })
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create the access key'
  },
})

const revoke = useMutation({
  mutationFn: (tokenId: string) => revokeServiceAccountToken(props.account.id, tokenId),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: serviceAccountKeys.tokens(props.account.id) }),
})

async function copySecret() {
  copyError.value = ''
  const result = await writeClipboardText(revealedSecret.value)
  if (result === 'unavailable') {
    copyError.value = 'Copy is unavailable. Select the key and copy it manually.'
    return
  }
  if (result === 'failed') {
    copied.value = false
    copyError.value = 'Copy failed. Select the key and copy it manually.'
    return
  }
  copied.value = true
  window.setTimeout(() => {
    copied.value = false
  }, 1800)
}

function closeSecret() {
  revealedSecret.value = ''
  copied.value = false
  copyError.value = ''
}
</script>

<template>
  <section class="keys-panel" :aria-label="`Access keys for ${account.name}`">
    <div class="keys-header">
      <div>
        <h3 class="text-sm font-semibold">Access keys</h3>
        <p class="mt-1 text-xs text-muted-foreground">Use <strong>{{ account.username }}</strong> as the Docker username and a key as the password.</p>
      </div>
      <Button v-if="!formOpen" size="sm" @click="formOpen = true"><Plus :size="15" /> New key</Button>
    </div>

    <form v-if="formOpen" class="key-form" @submit.prevent="create.mutate()">
      <label class="field-label">
        Key name
        <Input v-model="name" required autofocus placeholder="Production pipeline" />
      </label>
      <label class="field-label">
        Expiration <span class="text-muted-foreground">(optional)</span>
        <Input v-model="expiresAt" type="datetime-local" />
      </label>
      <p v-if="error" class="error-text">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" @click="formOpen = false">Cancel</Button>
        <Button type="submit" size="sm" :disabled="create.isPending.value">Create key</Button>
      </div>
    </form>

    <div v-if="revealedSecret" class="secret-reveal">
      <div class="reveal-copy">
        <ShieldAlert :size="18" />
        <div>
          <p class="text-sm font-semibold">Copy this key now</p>
          <p class="mt-1 text-xs text-muted-foreground">For security, it cannot be displayed again.</p>
        </div>
        <Button variant="ghost" size="icon" aria-label="Close revealed key" @click="closeSecret"><X :size="16" /></Button>
      </div>
      <div class="secret-value">
        <code>{{ revealedSecret }}</code>
        <Button size="sm" @click="copySecret">
          <Check v-if="copied" :size="15" />
          <Copy v-else :size="15" />
          {{ copied ? 'Copied' : 'Copy' }}
        </Button>
      </div>
      <p v-if="copyError" class="error-text" role="alert">{{ copyError }}</p>
    </div>

    <div v-if="keys.isLoading.value" class="keys-empty">Loading keys…</div>
    <div v-else-if="!pageItems(keys.data.value).length" class="keys-empty">
      <KeyRound :size="18" />
      <span>No keys linked to this service account.</span>
    </div>
    <div v-else class="key-list">
      <div v-for="token in pageItems(keys.data.value)" :key="token.id" class="key-row">
        <div>
          <p class="text-sm font-medium">{{ token.name }}</p>
          <p class="mt-1 font-mono text-xs text-muted-foreground">grm_{{ token.publicId }}_••••••••</p>
        </div>
        <div class="key-meta">
          <Badge :tone="token.revokedAt || (token.expiresAt && new Date(token.expiresAt).getTime() <= currentTime) ? undefined : 'success'">
            {{ token.revokedAt ? 'Revoked' : token.expiresAt && new Date(token.expiresAt).getTime() <= currentTime ? 'Expired' : 'Active' }}
          </Badge>
          <span v-if="token.expiresAt" class="text-xs text-muted-foreground">Expires {{ new Date(token.expiresAt).toLocaleString() }}</span>
          <span class="text-xs text-muted-foreground">Last used {{ token.lastUsedAt ? new Date(token.lastUsedAt).toLocaleString() : 'never' }}</span>
          <Button
            v-if="!token.revokedAt"
            variant="ghost"
            size="icon"
            aria-label="Revoke access key"
            @click="revoke.mutate(token.id)"
          >
            <Trash2 :size="15" />
          </Button>
        </div>
      </div>
    </div>
    <PaginationControls
      :page="pagination.page.value"
      :has-previous="pagination.hasPrevious.value"
      :has-next="Boolean(keys.data.value?.nextCursor)"
      :disabled="keys.isFetching.value"
      @previous="pagination.previous()"
      @next="pagination.next(keys.data.value?.nextCursor)"
    />
  </section>
</template>

<style scoped>
.keys-panel {
  border-top: 1px solid var(--border);
  background: rgba(0, 0, 0, 0.18);
  padding: 1rem;
}

.keys-header,
.key-row,
.key-meta,
.reveal-copy,
.secret-value {
  display: flex;
  align-items: center;
}

.keys-header,
.key-row,
.secret-value {
  justify-content: space-between;
  gap: 1rem;
}

.key-form,
.secret-reveal {
  margin-top: 1rem;
  border: 1px solid var(--border);
  border-radius: 0.7rem;
  padding: 1rem;
}

.key-form {
  display: grid;
  gap: 0.8rem;
}

.secret-reveal {
  border-color: color-mix(in srgb, var(--warning) 38%, transparent);
  background: color-mix(in srgb, var(--warning) 7%, transparent);
}

.reveal-copy {
  gap: 0.65rem;
  color: #e3c17d;
}

.reveal-copy > :last-child {
  margin-left: auto;
}

.secret-value {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  margin-top: 0.8rem;
}

.secret-value code {
  display: block;
  min-width: 0;
  overflow-wrap: anywhere;
  border: 1px solid var(--border);
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.22);
  padding: 0.7rem 0.8rem;
  color: var(--accent);
  font-size: 0.75rem;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

.key-list {
  margin-top: 0.75rem;
}

.key-row {
  min-height: 3.6rem;
  border-top: 1px solid var(--border);
}

.key-meta {
  gap: 0.7rem;
}

.keys-empty {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-height: 3.5rem;
  margin-top: 0.75rem;
  border-top: 1px solid var(--border);
  color: var(--muted-foreground);
  font-size: 0.8rem;
}

@media (max-width: 700px) {
  .keys-header,
  .key-row {
    align-items: stretch;
    flex-direction: column;
  }

  .secret-value {
    grid-template-columns: 1fr;
  }

  .secret-value :deep(.grom-button) {
    width: 100%;
    min-height: 3rem;
  }

  .key-meta {
    flex-wrap: wrap;
  }
}
</style>
