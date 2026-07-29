<script setup lang="ts">
import { completePasswordReset } from '@/modules/auth/api/session'
import { APIError } from '@/shared/api/client'
import { GromBrand } from '@/shared/components/brand'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { ROUTES } from '@/shared/constants'
import { useMutation } from '@tanstack/vue-query'
import { CheckCircle2, KeyRound } from '@lucide/vue'
import { ref } from 'vue'

const token = new globalThis.URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref(token ? '' : 'This password reset link is incomplete.')
const completed = ref(false)

const reset = useMutation({
  mutationFn: completePasswordReset,
  onSuccess: () => {
    completed.value = true
    error.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    window.history.replaceState(null, '', ROUTES.resetPassword)
  },
  onError: (caught) => {
    error.value = caught instanceof APIError
      ? caught.message
      : 'This password reset link is invalid or has expired.'
  },
})

function submit() {
  error.value = ''
  if (!token) {
    error.value = 'This password reset link is incomplete.'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'New password and confirmation do not match.'
    return
  }
  reset.mutate({ token, newPassword: newPassword.value })
}
</script>

<template>
  <main class="reset-page">
    <section class="reset-card">
      <GromBrand :crest-size="54" />

      <div v-if="completed" class="completed-state">
        <div class="completed-icon"><CheckCircle2 :size="25" /></div>
        <div>
          <h1>Password reset</h1>
          <p>Your new password is ready. All previous sessions have been signed out.</p>
        </div>
        <RouterLink :to="ROUTES.signIn" class="signin-link">Continue to sign in</RouterLink>
      </div>

      <template v-else>
        <div class="reset-heading">
          <KeyRound :size="23" class="text-accent" />
          <div>
            <h1>Choose a new password</h1>
            <p>Use at least 12 characters. This link can only be used once.</p>
          </div>
        </div>

        <form class="form-stack" @submit.prevent="submit">
          <label class="field-label">
            New password
            <Input v-model="newPassword" type="password" autocomplete="new-password" required minlength="12" />
          </label>
          <label class="field-label">
            Confirm new password
            <Input v-model="confirmPassword" type="password" autocomplete="new-password" required minlength="12" />
          </label>
          <p v-if="error" class="error-text" role="alert">{{ error }}</p>
          <Button type="submit" class="w-full" :loading="reset.isPending.value">Reset password</Button>
        </form>
      </template>
    </section>
  </main>
</template>

<style scoped>
.reset-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  padding: 2rem 1.25rem;
}

.reset-card {
  width: min(100%, 29rem);
  border: 1px solid var(--border-brand);
  border-radius: 1rem;
  background: color-mix(in srgb, var(--surface) 94%, transparent);
  padding: 2rem;
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.38);
}

.reset-heading,
.completed-state {
  margin-top: 2rem;
}

.reset-heading {
  display: flex;
  align-items: flex-start;
  gap: 0.8rem;
  margin-bottom: 1.5rem;
}

.reset-heading h1,
.completed-state h1 {
  margin: 0;
  font-size: 1.35rem;
  font-weight: 720;
}

.reset-heading p,
.completed-state p {
  margin-top: 0.45rem;
  color: var(--muted-foreground);
  font-size: 0.88rem;
  line-height: 1.55;
}

.completed-state {
  display: grid;
  gap: 1.25rem;
  text-align: center;
}

.completed-icon {
  display: grid;
  width: 3rem;
  height: 3rem;
  margin: 0 auto;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--accent) 35%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, var(--accent) 10%, transparent);
  color: #bdd66c;
}

.signin-link {
  color: #c9df6c;
  font-size: 0.9rem;
  font-weight: 680;
  text-decoration: none;
}

.signin-link:hover {
  text-decoration: underline;
}

@media (max-width: 560px) {
  .reset-page {
    align-items: start;
    padding-top: max(2rem, env(safe-area-inset-top));
  }

  .reset-card {
    border: 0;
    background: transparent;
    padding: 1rem 0;
    box-shadow: none;
  }
}
</style>
