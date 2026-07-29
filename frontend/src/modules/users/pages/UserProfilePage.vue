<script setup lang="ts">
import { changePassword } from '@/modules/auth/api/session'
import { useSessionStore } from '@/modules/auth/store/session'
import { APIError } from '@/shared/api/client'
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { Input } from '@/shared/components/ui/input'
import { useMutation } from '@tanstack/vue-query'
import { CheckCircle2, KeyRound, UserRound } from '@lucide/vue'
import { ref } from 'vue'

const session = useSessionStore()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const success = ref(false)

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
            <dd><Badge :tone="session.user?.systemAdmin ? 'success' : 'neutral'">{{ session.user?.systemAdmin ? 'Installation administrator' : 'User' }}</Badge></dd>
          </div>
          <div>
            <dt>Member since</dt>
            <dd>{{ session.user?.createdAt ? new Date(session.user.createdAt).toLocaleDateString() : '—' }}</dd>
          </div>
        </dl>
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
