<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import type { User } from '@/shared/api/models'
import { Badge } from '@/shared/components/ui/badge'
import { ActionButton, Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { writeClipboardText } from '@/shared/lib/clipboard'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Check, Copy, KeyRound, Plus, Search, ShieldAlert, UserRound, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { createUser, createUserPasswordResetLink, listUsers, userKeys } from '../api/users'

const queryClient = useQueryClient()
const users = useQuery({ queryKey: userKeys.all, queryFn: listUsers })
const modalOpen = ref(false)
const email = ref('')
const username = ref('')
const password = ref('')
const systemAdmin = ref(false)
const error = ref('')
const resetTarget = ref<User | null>(null)
const resetLink = ref('')
const resetExpiresAt = ref('')
const copied = ref(false)
const copyError = ref('')
const searchQuery = ref('')
const userCount = computed(() => users.data.value?.length ?? 0)
const filteredUsers = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  if (!query) return users.data.value ?? []

  return (users.data.value ?? []).filter((user) =>
    user.username.toLocaleLowerCase().includes(query) || user.email.toLocaleLowerCase().includes(query),
  )
})

const create = useMutation({
  mutationFn: createUser,
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: userKeys.all })
    modalOpen.value = false
    email.value = ''
    username.value = ''
    password.value = ''
    systemAdmin.value = false
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create user'
  },
})

const createResetLink = useMutation({
  mutationFn: (userId: string) => createUserPasswordResetLink(userId),
  onSuccess: (result) => {
    resetLink.value = result.url
    resetExpiresAt.value = result.expiresAt
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not reset the password'
  },
})

function openReset(user: User) {
  resetTarget.value = user
  resetLink.value = ''
  resetExpiresAt.value = ''
  copied.value = false
  copyError.value = ''
  error.value = ''
}

function closeReset() {
  resetTarget.value = null
  resetLink.value = ''
  resetExpiresAt.value = ''
  copied.value = false
  copyError.value = ''
}

async function copyResetLink() {
  copyError.value = ''
  const result = await writeClipboardText(resetLink.value)
  if (result === 'unavailable') {
    copyError.value = 'Copy is unavailable. Select the reset link and copy it manually.'
    return
  }
  if (result === 'failed') {
    copied.value = false
    copyError.value = 'Copy failed. Select the reset link and copy it manually.'
    return
  }
  copied.value = true
}
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Human identities</p>
        <h1 class="page-title">Users</h1>
        <p class="page-description">Create people who can access the management UI and be assigned to projects.</p>
      </div>
      <ActionButton @click="modalOpen = true"><Plus :size="17" /> New user</ActionButton>
    </header>

    <div class="table-shell">
      <div class="list-toolbar">
        <div>
          <h2>All users</h2>
          <p>People with access to the management interface.</p>
        </div>
        <div class="list-toolbar-actions">
          <div class="list-search">
            <Search :size="16" aria-hidden="true" />
            <Input
              v-model="searchQuery"
              type="search"
              placeholder="Search users"
              aria-label="Search users"
            />
          </div>
          <span v-if="!users.isLoading.value" class="list-count">
            {{ searchQuery.trim() ? `${filteredUsers.length} of ${userCount}` : `${userCount} total` }}
          </span>
        </div>
      </div>

      <div v-if="!users.isLoading.value && !userCount" class="empty-state list-empty-state">
        <div>
          <UserRound class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No users yet</p>
        </div>
      </div>
      <div v-else-if="!users.isLoading.value && !filteredUsers.length" class="empty-state list-empty-state">
        <div>
          <Search class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No matching users</p>
          <p class="mt-1 text-sm">Try a different username or email.</p>
        </div>
      </div>
      <template v-else>
        <div v-for="user in filteredUsers" :key="user.id" class="data-row">
          <div class="flex items-center gap-3">
            <div class="avatar"><UserRound :size="15" /></div>
            <div><p class="text-sm font-semibold">{{ user.username }}</p><p class="mt-1 text-xs text-muted-foreground">{{ user.email }}</p></div>
          </div>
          <p class="text-xs text-muted-foreground">Created {{ new Date(user.createdAt).toLocaleDateString() }}</p>
          <div class="user-actions">
            <Badge :tone="user.systemAdmin ? 'success' : 'neutral'">{{ user.systemAdmin ? 'System admin' : 'User' }}</Badge>
            <Button size="sm" variant="outline" @click="openReset(user)"><KeyRound :size="15" /> Reset password</Button>
          </div>
        </div>
      </template>
    </div>

    <div v-if="modalOpen" class="modal-backdrop" @click.self="modalOpen = false">
      <form class="modal form-stack" @submit.prevent="create.mutate({ email, username, password, systemAdmin })">
        <div class="flex items-start justify-between"><div><h2 class="text-lg font-semibold">Create user</h2><p class="mt-1 text-sm text-muted-foreground">They can change access through project memberships.</p></div><Button variant="ghost" size="icon" @click="modalOpen = false"><X :size="18" /></Button></div>
        <label class="field-label">Email<Input v-model="email" type="email" required /></label>
        <label class="field-label">Username<Input v-model="username" required placeholder="alex" /></label>
        <label class="field-label">Initial password<Input v-model="password" type="password" autocomplete="new-password" required minlength="8" /></label>
        <label class="flex items-center gap-2 text-sm text-muted-foreground"><input v-model="systemAdmin" type="checkbox" /> Installation administrator</label>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" @click="modalOpen = false">Cancel</Button><ActionButton type="submit">Create user</ActionButton></div>
      </form>
    </div>

    <div v-if="resetTarget" class="modal-backdrop" @click.self="closeReset">
      <section class="modal form-stack" role="dialog" aria-modal="true" aria-labelledby="reset-password-title">
        <div class="flex items-start justify-between">
          <div>
            <h2 id="reset-password-title" class="text-lg font-semibold">Reset {{ resetTarget.username }}'s password</h2>
            <p class="mt-1 text-sm text-muted-foreground">Generate a single-use link that lets this user choose a new password. Existing sessions end when the link is used.</p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close password reset" @click="closeReset"><X :size="18" /></Button>
        </div>

        <template v-if="!resetLink">
          <p v-if="error" class="error-text" role="alert">{{ error }}</p>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" @click="closeReset">Cancel</Button>
            <Button :loading="createResetLink.isPending.value" @click="createResetLink.mutate(resetTarget.id)">
              Generate reset link
            </Button>
          </div>
        </template>

        <div v-else class="reset-secret">
          <div class="reveal-heading">
            <ShieldAlert :size="18" />
            <div>
              <p class="text-sm font-semibold">Copy this reset link now</p>
              <p class="mt-1 text-xs text-muted-foreground">It expires {{ new Date(resetExpiresAt).toLocaleString() }} and cannot be displayed again.</p>
            </div>
          </div>
          <code>{{ resetLink }}</code>
          <p v-if="copyError" class="error-text" role="alert">{{ copyError }}</p>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" @click="closeReset">Done</Button>
            <Button @click="copyResetLink">
              <Check v-if="copied" :size="15" />
              <Copy v-else :size="15" />
              {{ copied ? 'Copied' : 'Copy link' }}
            </Button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
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

.list-toolbar + .data-row {
  border-top: 0;
}

.user-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.65rem;
}

.reset-secret {
  display: grid;
  gap: 1rem;
  border: 1px solid color-mix(in srgb, var(--warning) 38%, transparent);
  border-radius: 0.7rem;
  background: color-mix(in srgb, var(--warning) 7%, transparent);
  padding: 1rem;
}

.reveal-heading {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  color: #e3c17d;
}

.reset-secret code {
  overflow-wrap: anywhere;
  border: 1px solid var(--border);
  border-radius: 0.55rem;
  background: rgba(0, 0, 0, 0.22);
  padding: 0.8rem;
  color: var(--foreground);
}

@media (max-width: 800px) {
  .user-actions {
    align-items: flex-end;
    flex-direction: column;
  }
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
}
</style>
