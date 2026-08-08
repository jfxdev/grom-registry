<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import { useSessionStore } from '@/modules/auth/store/session'
import type { User } from '@/shared/api/models'
import { ActionButton, Button, DeleteButton } from '@/shared/components/ui/button'
import { Badge } from '@/shared/components/ui/badge'
import { DropdownMenu } from '@/shared/components/ui/dropdown-menu'
import { Input } from '@/shared/components/ui/input'
import { PaginationControls } from '@/shared/components/ui/pagination'
import { writeClipboardText } from '@/shared/lib/clipboard'
import { pageItems, useCursorPagination } from '@/shared/lib/pagination'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { Check, CircleCheck, CircleOff, Copy, Eye, KeyRound, Plus, Search, ShieldAlert, UserRound, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { createUser, createUserPasswordResetLink, disableUser, listUsers, promoteUserToSystemAdmin, promoteUserToSystemViewer, userKeys } from '../api/users'

const queryClient = useQueryClient()
const session = useSessionStore()
const pagination = useCursorPagination()
const users = useQuery({ queryKey: computed(() => [...userKeys.all, pagination.cursor.value]), queryFn: () => listUsers(pagination.cursor.value) })
const modalOpen = ref(false)
const email = ref('')
const username = ref('')
const error = ref('')
const registrationLink = ref('')
const registrationExpiresAt = ref('')
const registrationCopied = ref(false)
const registrationCopyError = ref('')
const resetTarget = ref<User | null>(null)
const resetLink = ref('')
const resetExpiresAt = ref('')
const copied = ref(false)
const copyError = ref('')
const searchQuery = ref('')
const disableTarget = ref<User | null>(null)
const disableError = ref('')
const promoteTarget = ref<User | null>(null)
const promoteError = ref('')
const viewerTarget = ref<User | null>(null)
const viewerError = ref('')
const userCount = computed(() => pageItems(users.data.value).length)
const filteredUsers = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  if (!query) return pageItems(users.data.value)

  return pageItems(users.data.value).filter((user) =>
    user.username.toLocaleLowerCase().includes(query) || user.email.toLocaleLowerCase().includes(query),
  )
})

const create = useMutation({
  mutationFn: createUser,
  onSuccess: async (created) => {
    await queryClient.invalidateQueries({ queryKey: userKeys.all })
    modalOpen.value = false
    email.value = ''
    username.value = ''
    registrationLink.value = created.registrationLink.url
    registrationExpiresAt.value = created.registrationLink.expiresAt
    registrationCopied.value = false
    registrationCopyError.value = ''
  },
  onError: (caught) => {
    error.value = createUserError(caught)
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

const disable = useMutation({
  mutationFn: (userId: string) => disableUser(userId),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: userKeys.all })
    disableTarget.value = null
  },
  onError: (caught) => {
    disableError.value = caught instanceof APIError ? caught.message : 'Could not disable the user'
  },
})

const promote = useMutation({
  mutationFn: (userId: string) => promoteUserToSystemAdmin(userId),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: userKeys.all })
    promoteTarget.value = null
  },
  onError: (caught) => {
    promoteError.value = caught instanceof APIError ? caught.message : 'Could not promote the user'
  },
})

const promoteViewer = useMutation({
  mutationFn: (userId: string) => promoteUserToSystemViewer(userId),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: userKeys.all })
    viewerTarget.value = null
  },
  onError: (caught) => {
    viewerError.value = caught instanceof APIError ? caught.message : 'Could not promote the user to viewer'
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

function openDisable(user: User) {
  disableTarget.value = user
  disableError.value = ''
}

function closeDisable() {
  if (!disable.isPending.value) disableTarget.value = null
  disableError.value = ''
}

function openPromote(user: User) {
  promoteTarget.value = user
  promoteError.value = ''
}

function closePromote() {
  if (!promote.isPending.value) promoteTarget.value = null
  promoteError.value = ''
}

function openViewer(user: User) {
  viewerTarget.value = user
  viewerError.value = ''
}

function chooseRole(user: User, role: string) {
  if (role === 'administrator') openPromote(user)
  if (role === 'viewer') openViewer(user)
}

function roleLabel(user: User) {
  if (user.systemAdmin) return 'Administrator'
  if (user.systemViewer) return 'Viewer'
  return 'User'
}

function closeViewer() {
  if (!promoteViewer.isPending.value) viewerTarget.value = null
  viewerError.value = ''
}

function createUserError(caught: unknown) {
  if (!(caught instanceof APIError)) return 'Could not create user'

  if (caught.code === 'username_taken') {
    return 'This username is already in use.'
  }
  if (caught.code === 'email_taken') {
    return 'This email address is already in use.'
  }
  return 'Could not create user. Please try again.'
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

async function copyRegistrationLink() {
  registrationCopyError.value = ''
  const result = await writeClipboardText(registrationLink.value)
  if (result === 'unavailable') {
    registrationCopyError.value = 'Copy is unavailable. Select the registration link and copy it manually.'
    return
  }
  if (result === 'failed') {
    registrationCopied.value = false
    registrationCopyError.value = 'Copy failed. Select the registration link and copy it manually.'
    return
  }
  registrationCopied.value = true
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

      <div v-if="users.isLoading.value" class="empty-state list-empty-state" role="status">
        <p class="text-sm text-muted-foreground">Loading users…</p>
      </div>
      <div v-else-if="!userCount" class="empty-state list-empty-state">
        <div>
          <UserRound class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No users yet</p>
        </div>
      </div>
      <div v-else-if="!filteredUsers.length" class="empty-state list-empty-state">
        <div>
          <Search class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No matching users</p>
          <p class="mt-1 text-sm">Try a different username or email.</p>
        </div>
      </div>
      <template v-else>
        <div class="users-table-head" aria-hidden="true">
          <span>User</span><span>Created</span><span>Status</span><span>Role</span><span>Access</span><span>Actions</span>
        </div>
        <div v-for="user in filteredUsers" :key="user.id" class="data-row user-row">
          <div class="user-identity flex items-center gap-3">
            <div class="avatar"><UserRound :size="15" /></div>
            <div>
              <p class="flex items-center gap-2 text-sm font-semibold"><span>{{ user.username }}</span><Badge class="gap-1" :tone="user.systemAdmin ? 'warning' : 'neutral'"><ShieldAlert v-if="user.systemAdmin" :size="12" /><Eye v-else-if="user.systemViewer" :size="12" /><UserRound v-else :size="12" />{{ roleLabel(user) }}</Badge></p>
              <p class="mt-1 text-xs text-muted-foreground">{{ user.email }}</p>
            </div>
          </div>
          <p class="user-created text-xs text-muted-foreground">{{ new Date(user.createdAt).toLocaleDateString() }}</p>
          <span class="user-status" :class="{ 'user-status-inactive': user.disabledAt }" :aria-label="user.disabledAt ? 'Inactive user' : 'Active user'">
            <CircleOff v-if="user.disabledAt" :size="17" aria-hidden="true" />
            <CircleCheck v-else :size="17" aria-hidden="true" />
          </span>
          <div class="user-role-cell">
            <DropdownMenu v-if="!user.disabledAt && !user.systemAdmin" label="Set role" v-bind="{ ariaLabel: `Change role for ${user.username}` }">
              <template #icon><ShieldAlert :size="15" /></template>
              <template #default="{ close }">
                <button v-if="!user.systemViewer" type="button" class="dropdown-menu-item" role="menuitem" @click="close(); chooseRole(user, 'viewer')"><Eye :size="15" /> Viewer</button>
                <button type="button" class="dropdown-menu-item" role="menuitem" @click="close(); chooseRole(user, 'administrator')"><ShieldAlert :size="15" /> Administrator</button>
              </template>
            </DropdownMenu>
          </div>
          <div class="user-access-cell">
            <Button v-if="!user.disabledAt" size="sm" variant="outline" @click="openReset(user)"><KeyRound :size="15" /> Reset password</Button>
          </div>
          <div class="user-actions">
            <DeleteButton v-if="!user.disabledAt" size="sm" :disabled="user.id === session.user?.id" :aria-label="`Disable user ${user.username}`" @click="openDisable(user)" />
          </div>
        </div>
      </template>
      <PaginationControls
        :page="pagination.page.value"
        :page-count="users.data.value?.pageCount ?? 0"
        :has-previous="pagination.hasPrevious.value"
        :has-next="Boolean(users.data.value?.nextCursor)"
        :disabled="users.isFetching.value"
        @previous="pagination.previous()"
        @next="pagination.next(users.data.value?.nextCursor)"
      />
    </div>

    <div v-if="disableTarget" class="modal-backdrop" @click.self="closeDisable">
      <form class="modal form-stack" aria-labelledby="disable-user-title" @submit.prevent="disable.mutate(disableTarget!.id)">
        <div class="flex items-start justify-between"><div><h2 id="disable-user-title" class="text-lg font-semibold">Disable {{ disableTarget.username }}</h2><p class="mt-1 text-sm text-muted-foreground">This revokes the user’s active sessions and blocks future sign-in.</p></div><Button variant="ghost" size="icon" aria-label="Close disable user" @click="closeDisable"><X :size="18" /></Button></div>
        <p v-if="disableError" class="error-text" role="alert">{{ disableError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" :disabled="disable.isPending.value" @click="closeDisable">Cancel</Button><Button type="submit" variant="danger" :loading="disable.isPending.value">Disable user</Button></div>
      </form>
    </div>

    <div v-if="modalOpen" class="modal-backdrop" @click.self="modalOpen = false">
      <form class="modal form-stack" @submit.prevent="create.mutate({ email, username })">
        <div class="flex items-start justify-between"><div><h2 class="text-lg font-semibold">Create user</h2><p class="mt-1 text-sm text-muted-foreground">They will receive a one-time registration link to set their password. New users have standard access.</p></div><Button variant="ghost" size="icon" @click="modalOpen = false"><X :size="18" /></Button></div>
        <label class="field-label">Email<Input v-model="email" type="email" required /></label>
        <label class="field-label">Username<Input v-model="username" required placeholder="alex" /></label>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" @click="modalOpen = false">Cancel</Button><ActionButton type="submit">Create user</ActionButton></div>
      </form>
    </div>

    <div v-if="registrationLink" class="modal-backdrop">
      <section class="modal form-stack reveal-modal" role="dialog" aria-modal="true" aria-labelledby="registration-link-title">
        <div class="reveal-heading">
          <ShieldAlert :size="19" aria-hidden="true" />
          <div><h2 id="registration-link-title" class="text-lg font-semibold">Copy this registration link now</h2><p class="mt-1 text-sm text-muted-foreground">It expires {{ new Date(registrationExpiresAt).toLocaleString() }} and cannot be displayed again.</p></div>
          <Button variant="ghost" size="icon" aria-label="Close registration link" @click="registrationLink = ''"><X :size="18" /></Button>
        </div>
        <div class="reveal-value"><code>{{ registrationLink }}</code></div>
        <p v-if="registrationCopyError" class="error-text" role="alert">{{ registrationCopyError }}</p>
        <div class="reveal-actions"><Button variant="outline" @click="registrationLink = ''">Done</Button><Button @click="copyRegistrationLink"><Check v-if="registrationCopied" :size="15" /><Copy v-else :size="15" />{{ registrationCopied ? 'Copied' : 'Copy link' }}</Button></div>
      </section>
    </div>

    <div v-if="promoteTarget" class="modal-backdrop" @click.self="closePromote">
      <form class="modal form-stack" aria-labelledby="promote-user-title" @submit.prevent="promote.mutate(promoteTarget!.id)">
        <div class="flex items-start justify-between"><div><h2 id="promote-user-title" class="text-lg font-semibold">Make {{ promoteTarget.username }} an administrator?</h2><p class="mt-1 text-sm text-muted-foreground">Installation administrators can manage users, service accounts, and projects.</p></div><Button variant="ghost" size="icon" aria-label="Close administrator promotion" @click="closePromote"><X :size="18" /></Button></div>
        <p v-if="promoteError" class="error-text" role="alert">{{ promoteError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" :disabled="promote.isPending.value" @click="closePromote">Cancel</Button><Button type="submit" :loading="promote.isPending.value">Make administrator</Button></div>
      </form>
    </div>

    <div v-if="viewerTarget" class="modal-backdrop" @click.self="closeViewer">
      <form class="modal form-stack" aria-labelledby="promote-viewer-title" @submit.prevent="promoteViewer.mutate(viewerTarget!.id)">
        <div class="flex items-start justify-between"><div><h2 id="promote-viewer-title" class="text-lg font-semibold">Make {{ viewerTarget.username }} a viewer?</h2><p class="mt-1 text-sm text-muted-foreground">Viewers have read-only access and need an explicit project membership to view registry information.</p></div><Button variant="ghost" size="icon" aria-label="Close viewer promotion" @click="closeViewer"><X :size="18" /></Button></div>
        <p v-if="viewerError" class="error-text" role="alert">{{ viewerError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" :disabled="promoteViewer.isPending.value" @click="closeViewer">Cancel</Button><Button type="submit" :loading="promoteViewer.isPending.value">Make viewer</Button></div>
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
          <div class="reveal-value"><code>{{ resetLink }}</code></div>
          <p v-if="copyError" class="error-text" role="alert">{{ copyError }}</p>
          <div class="reveal-actions">
            <Button variant="outline" @click="closeReset">Done</Button>
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

.users-table-head + .data-row {
  border-top: 0;
}

.users-table-head,
.user-row {
  grid-template-columns: minmax(17rem, 1fr) 8.5rem 3rem 9rem 12.5rem 9rem;
}

.users-table-head {
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

.user-row {
  gap: 1rem;
}

.user-created {
  white-space: nowrap;
}

.user-role-cell,
.user-access-cell,
.user-actions {
  display: flex;
  align-items: center;
}

.user-actions {
  justify-content: flex-end;
}

.user-status {
  display: inline-grid;
  flex: none;
  place-items: center;
  color: var(--accent);
}

.user-status-inactive {
  color: var(--muted-foreground);
}

@media (max-width: 980px) {
  .users-table-head { display: none; }
  .user-row { grid-template-columns: minmax(15rem, 1fr) 8rem auto; }
  .user-role-cell { grid-column: 2; }
  .user-access-cell { grid-column: 1 / span 2; }
  .user-actions { grid-column: 3; grid-row: 1 / span 2; }
}

@media (max-width: 640px) {
  .user-row { grid-template-columns: minmax(0, 1fr) auto; gap: 0.75rem; }
  .user-created { grid-column: 1; }
  .user-status { grid-column: 2; grid-row: 1; }
  .user-role-cell { grid-column: 1; }
  .user-access-cell { grid-column: 1; }
  .user-actions { grid-column: 2; grid-row: 2 / span 2; align-self: center; }
}

.reveal-modal {
  width: min(100%, 43rem);
  max-height: calc(100dvh - 2rem);
  overflow-y: auto;
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
  align-items: flex-start;
  gap: 0.65rem;
  color: #e3c17d;
}

.reveal-heading > :last-child {
  margin-left: auto;
}

.reveal-heading svg {
  flex: none;
  margin-top: 0.12rem;
}

.reveal-value {
  max-width: 100%;
  overflow: auto;
  overflow-wrap: anywhere;
  border: 1px solid var(--border);
  border-radius: 0.55rem;
  background: rgba(0, 0, 0, 0.22);
  padding: 0.8rem;
}

.reveal-value code {
  display: block;
  color: var(--foreground);
  font-size: 0.78rem;
  line-height: 1.65;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  word-break: break-word;
}

.reveal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
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

  .reveal-modal {
    width: 100%;
    max-height: calc(100dvh - 1.5rem);
  }

  .reveal-heading {
    gap: 0.5rem;
  }

  .reveal-heading h2 {
    font-size: 1rem;
  }

  .reveal-value {
    padding: 0.75rem;
  }

  .reveal-value code {
    font-size: 0.71rem;
    line-height: 1.55;
  }

  .reveal-actions {
    flex-direction: column-reverse;
  }

  .reveal-actions :deep(.grom-button) {
    width: 100%;
    min-height: 3rem;
  }
}
</style>
