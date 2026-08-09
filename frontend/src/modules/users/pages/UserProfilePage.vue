<script setup lang="ts">
import { changePassword } from '@/modules/auth/api/session'
import { useSessionStore } from '@/modules/auth/store/session'
import { createViewerRegistryToken, listViewerRegistryTokens, revokeViewerRegistryToken, viewerRegistryTokenKeys } from '@/modules/users/api/viewerRegistryTokens'
import { APIError } from '@/shared/api/client'
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { Input } from '@/shared/components/ui/input'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { CheckCircle2, Copy, KeyRound, Trash2, UserRound } from '@lucide/vue'
import { computed, ref } from 'vue'

const session = useSessionStore()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const success = ref(false)
const registryTokenName = ref('')
const registryTokenError = ref('')
const revealedRegistryToken = ref('')
const queryClient = useQueryClient()
const isViewer = computed(() => session.user?.systemViewer === true)
const registryTokens = useQuery({ queryKey: viewerRegistryTokenKeys.all, queryFn: listViewerRegistryTokens, enabled: isViewer })
const hasActiveRegistryToken = computed(() => (registryTokens.data.value?.length ?? 0) > 0)

const change = useMutation({
  mutationFn: changePassword,
  onSuccess: () => {
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    error.value = ''
    success.value = true
  },
  onError: (caught) => {
    success.value = false
    error.value = caught instanceof APIError ? caught.message : 'Could not change the password'
  },
})

const createRegistryToken = useMutation({
  mutationFn: () => createViewerRegistryToken({ name: registryTokenName.value }),
  onSuccess: (created) => {
    revealedRegistryToken.value = created.secret
    registryTokenName.value = ''
    registryTokenError.value = ''
    void queryClient.invalidateQueries({ queryKey: viewerRegistryTokenKeys.all })
  },
  onError: (caught) => {
    registryTokenError.value = caught instanceof APIError && caught.code === 'viewer_token_exists'
      ? 'Revoke the active registry token before creating another one.'
      : caught instanceof APIError ? caught.message : 'Could not create the registry token'
  },
})

const revokeRegistryToken = useMutation({
  mutationFn: revokeViewerRegistryToken,
  onSuccess: () => void queryClient.invalidateQueries({ queryKey: viewerRegistryTokenKeys.all }),
})

function submit() {
  success.value = false
  error.value = ''
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'New password and confirmation do not match'
    return
  }
  change.mutate({
    currentPassword: currentPassword.value,
    newPassword: newPassword.value,
  })
}

async function copyRegistryToken() { await navigator.clipboard.writeText(revealedRegistryToken.value) }
</script>

<template>
  <div class="page-shell profile-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Your account</p>
        <h1 class="page-title">Profile</h1>
        <p class="page-description">Review your account details and keep your sign-in password secure.</p>
      </div>
    </header>

    <div class="profile-grid">
      <Card>
        <div class="profile-heading">
          <div class="avatar profile-avatar"><UserRound :size="22" /></div>
          <div>
            <h2 class="text-lg font-semibold">{{ session.user?.username }}</h2>
            <p class="mt-1 text-sm text-muted-foreground">{{ session.user?.email }}</p>
          </div>
        </div>
        <dl class="profile-details">
          <div>
            <dt>Account type</dt>
            <dd><Badge :tone="session.user?.systemAdmin ? 'success' : 'neutral'">{{ session.user?.systemAdmin ? 'Installation administrator' : session.user?.systemViewer ? 'Installation viewer' : 'User' }}</Badge></dd>
          </div>
          <div>
            <dt>Member since</dt>
            <dd>{{ session.user?.createdAt ? new Date(session.user.createdAt).toLocaleDateString() : '—' }}</dd>
          </div>
        </dl>
      </Card>

      <Card v-if="isViewer">
        <div class="profile-heading">
          <KeyRound :size="22" class="text-accent" />
          <div><h2 class="text-lg font-semibold">Read-only registry tokens</h2><p class="mt-1 text-sm text-muted-foreground">Tokens can pull only from projects where you have explicit access.</p></div>
        </div>
        <form v-if="!hasActiveRegistryToken" class="form-stack mt-5" @submit.prevent="createRegistryToken.mutate()">
          <label class="field-label">Token name<Input v-model="registryTokenName" required maxlength="120" placeholder="Local Docker" /></label>
          <p v-if="registryTokenError" class="error-text" role="alert">{{ registryTokenError }}</p>
          <div class="flex justify-end"><Button type="submit" :loading="createRegistryToken.isPending.value">Create read-only token</Button></div>
        </form>
        <p v-else class="mt-5 text-sm text-muted-foreground">Revoke the active token before creating another one.</p>
        <div v-if="revealedRegistryToken" class="reveal-token" role="status"><p>Copy this token now. It will not be shown again.</p><code>{{ revealedRegistryToken }}</code><Button variant="outline" size="sm" type="button" @click="copyRegistryToken"><Copy :size="15" /> Copy token</Button></div>
        <p v-if="registryTokens.isLoading.value" class="mt-5 text-sm text-muted-foreground">Loading tokens…</p>
        <ul v-else class="token-list">
          <li v-for="token in registryTokens.data.value" :key="token.id"><span><strong>{{ token.name }}</strong><small>{{ token.lastUsedAt ? `Last used ${new Date(token.lastUsedAt).toLocaleDateString()}` : 'Not used yet' }}</small></span><Button variant="ghost" size="icon" :aria-label="`Revoke ${token.name}`" @click="revokeRegistryToken.mutate(token.id)"><Trash2 :size="16" /></Button></li>
        </ul>
      </Card>

      <Card>
        <div class="profile-heading">
          <KeyRound :size="22" class="text-accent" />
          <div>
            <h2 class="text-lg font-semibold">Change password</h2>
            <p class="mt-1 text-sm text-muted-foreground">Use at least 12 characters for the new password.</p>
          </div>
        </div>

        <form class="form-stack mt-5" @submit.prevent="submit">
          <label class="field-label">
            Current password
            <Input v-model="currentPassword" type="password" autocomplete="current-password" required minlength="8" />
          </label>
          <label class="field-label">
            New password
            <Input v-model="newPassword" type="password" autocomplete="new-password" required minlength="12" />
          </label>
          <label class="field-label">
            Confirm new password
            <Input v-model="confirmPassword" type="password" autocomplete="new-password" required minlength="12" />
          </label>
          <p v-if="error" class="error-text" role="alert">{{ error }}</p>
          <p v-if="success" class="success-message" role="status"><CheckCircle2 :size="16" /> Password changed successfully.</p>
          <div class="flex justify-end">
            <Button type="submit" :loading="change.isPending.value">Change password</Button>
          </div>
        </form>
      </Card>
    </div>
  </div>
</template>

<style scoped>
.profile-shell {
  max-width: 64rem;
}

.profile-grid {
  display: grid;
  grid-template-columns: minmax(15rem, 0.75fr) minmax(20rem, 1.25fr);
  gap: 1rem;
  align-items: start;
}

.profile-heading {
  display: flex;
  align-items: center;
  gap: 0.8rem;
}

.profile-avatar {
  width: 2.8rem;
  height: 2.8rem;
}

.profile-details {
  display: grid;
  gap: 1rem;
  margin-top: 1.5rem;
}

.profile-details > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}

.reveal-token { display: grid; gap: .7rem; margin-top: 1rem; padding: 1rem; border: 1px solid var(--border); border-radius: .65rem; background: var(--muted); }
.reveal-token code { overflow-wrap: anywhere; font-size: .78rem; }
.token-list { display: grid; gap: .5rem; margin-top: 1.25rem; padding: 0; list-style: none; }
.token-list li { display: flex; align-items: center; justify-content: space-between; gap: 1rem; border-top: 1px solid var(--border); padding-top: .7rem; }
.token-list span { display: grid; gap: .15rem; }.token-list small { color: var(--muted-foreground); }

.profile-details dt {
  color: var(--muted-foreground);
  font-size: 0.8rem;
}

.profile-details dd {
  margin: 0;
  font-size: 0.85rem;
}

.success-message {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  color: #bdd66c;
  font-size: 0.82rem;
}

@media (max-width: 760px) {
  .profile-grid {
    grid-template-columns: 1fr;
  }
}
</style>
