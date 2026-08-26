<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { APIError } from '@/shared/api/client'
import { CrystalDivider, GromBrand } from '@/shared/components/brand'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { ROUTES } from '@/shared/constants'
import { Eye, EyeOff, ShieldCheck } from '@lucide/vue'
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const session = useSessionStore()
const route = useRoute()
const router = useRouter()
const email = ref('admin@grom.local')
const password = ref('')
const error = ref('')
const loading = ref(false)
const showPassword = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await session.signIn(email.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ROUTES.projects
    await router.push(redirect)
  } catch (caught) {
    error.value = caught instanceof APIError ? caught.message : 'Sign in failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="signin-page">
    <section class="signin-surface">
      <span class="signin-corner signin-corner--top-left" aria-hidden="true" />
      <span class="signin-corner signin-corner--top-right" aria-hidden="true" />
      <span class="signin-corner signin-corner--bottom-right" aria-hidden="true" />
      <span class="signin-corner signin-corner--bottom-left" aria-hidden="true" />
      <GromBrand class="signin-brand" layout="stacked" :crest-size="82" />
      <CrystalDivider class="signin-divider" />

      <form class="signin-form" :aria-busy="loading" @submit.prevent="submit">
        <div class="signin-heading">
          <h1>Sign in</h1>
          <p>Manage projects, repositories and access keys.</p>
          <p class="signin-invitation">New users are invited by an administrator.</p>
        </div>

        <label class="field-label">
          Email
          <Input
            v-model="email"
            type="email"
            autocomplete="email"
            :aria-invalid="error ? 'true' : undefined"
            required
          />
        </label>

        <div class="password-field">
          <label class="field-label">
            Password
            <Input
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              :aria-invalid="error ? 'true' : undefined"
              required
            />
          </label>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            class="password-toggle"
            :aria-label="showPassword ? 'Hide password' : 'Show password'"
            @click="showPassword = !showPassword"
          >
            <EyeOff v-if="showPassword" :size="18" />
            <Eye v-else :size="18" />
          </Button>
        </div>

        <p v-if="error" class="error-text" role="alert">{{ error }}</p>

        <Button type="submit" class="signin-submit" :loading="loading">
          {{ loading ? 'Signing in…' : 'Sign in' }}
        </Button>

        <div class="signin-compatibility">
          <ShieldCheck :size="16" aria-hidden="true" />
          <span>OCI compatible · Distribution powered</span>
        </div>
      </form>
    </section>
  </main>
</template>

<style scoped>
.signin-page {
  display: grid;
  min-height: 100svh;
  place-items: center;
  padding:
    max(1.5rem, env(safe-area-inset-top))
    max(1.5rem, env(safe-area-inset-right))
    max(1.5rem, env(safe-area-inset-bottom))
    max(1.5rem, env(safe-area-inset-left));
  background:
    radial-gradient(circle at 50% 24%, rgba(145, 173, 36, 0.11), transparent 23rem),
    linear-gradient(145deg, rgba(169, 120, 50, 0.025), transparent 42%),
    var(--background);
}

.signin-surface {
  --corner: 1.35rem;
  position: relative;
  display: flex;
  width: min(100%, 29rem);
  flex-direction: column;
  align-items: center;
  border: 1px solid var(--border-brand);
  padding: 2.5rem 2.35rem 2.1rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.018), transparent 24%),
    rgba(17, 21, 18, 0.94);
  box-shadow:
    0 28px 70px rgba(0, 0, 0, 0.42),
    inset 0 1px 0 rgba(255, 255, 255, 0.025);
  clip-path: polygon(
    var(--corner) 0,
    calc(100% - var(--corner)) 0,
    100% var(--corner),
    100% calc(100% - var(--corner)),
    calc(100% - var(--corner)) 100%,
    var(--corner) 100%,
    0 calc(100% - var(--corner)),
    0 var(--corner)
  );
}

.signin-surface::before {
  position: absolute;
  top: 0;
  left: var(--corner);
  width: 4.5rem;
  height: 1px;
  background: linear-gradient(90deg, var(--bronze), transparent);
  content: "";
}

.signin-corner {
  position: absolute;
  z-index: 2;
  width: calc(var(--corner) * 1.414214);
  height: 1px;
  background: var(--border-brand);
  pointer-events: none;
}

.signin-corner--top-left {
  top: var(--corner);
  left: 0;
  transform-origin: left center;
  transform: rotate(-45deg);
}

.signin-corner--top-right {
  top: 0;
  right: var(--corner);
  transform-origin: right center;
  transform: rotate(-135deg);
}

.signin-corner--bottom-right {
  right: var(--corner);
  bottom: 0;
  transform-origin: right center;
  transform: rotate(135deg);
}

.signin-corner--bottom-left {
  bottom: var(--corner);
  left: 0;
  transform-origin: left center;
  transform: rotate(45deg);
}

.signin-brand {
  position: relative;
  z-index: 1;
}

.signin-divider {
  margin: 1.1rem 0 1.2rem;
}

.signin-form {
  display: grid;
  width: 100%;
  gap: 1.1rem;
}

.signin-heading {
  margin-bottom: 0.25rem;
  text-align: center;
}

.signin-heading h1 {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 720;
  letter-spacing: -0.04em;
}

.signin-heading p {
  margin: 0.5rem auto 0;
  color: var(--muted-foreground);
  font-size: 0.86rem;
  line-height: 1.5;
}

.signin-heading .signin-invitation {
  margin-top: 0.35rem;
  color: color-mix(in srgb, var(--accent) 75%, var(--muted-foreground));
  font-size: 0.76rem;
}

.password-field {
  position: relative;
}

.password-field :deep(.grom-input) {
  padding-right: 3.2rem;
}

.password-toggle {
  position: absolute;
  right: 0.35rem;
  bottom: 0.28rem;
  width: 2.2rem;
  min-width: 2.2rem;
  height: 2.2rem;
  color: var(--muted-foreground);
}

.signin-submit {
  width: 100%;
  min-height: 3rem;
  margin-top: 0.15rem;
  font-size: 0.95rem;
}

.signin-compatibility {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.45rem;
  margin-top: 0.75rem;
  border-top: 1px solid var(--border);
  padding-top: 1.05rem;
  color: var(--muted-foreground);
  font-size: 0.72rem;
  text-align: center;
}

.signin-compatibility svg {
  color: var(--accent);
}

@media (max-width: 600px) {
  .signin-page {
    place-items: start center;
    padding:
      max(2.5rem, env(safe-area-inset-top))
      max(1.5rem, env(safe-area-inset-right))
      max(2rem, env(safe-area-inset-bottom))
      max(1.5rem, env(safe-area-inset-left));
  }

  .signin-surface {
    width: 100%;
    max-width: 28rem;
    border: 0;
    padding: 0;
    background: transparent;
    box-shadow: none;
    clip-path: none;
  }

  .signin-surface::before {
    display: none;
  }

  .signin-corner {
    display: none;
  }

  .signin-brand :deep(.grom-crest) {
    width: 58px;
    height: 58px;
  }

  .signin-divider {
    margin: 1rem 0 1.35rem;
  }

  .signin-form {
    gap: 1.2rem;
  }

  .signin-heading {
    margin-bottom: 0.55rem;
  }

  .signin-heading h1 {
    font-size: 1.8rem;
  }

  .signin-heading p {
    max-width: 20rem;
    font-size: 0.9rem;
  }

  .signin-submit {
    min-height: 3.15rem;
  }
}

@media (min-width: 601px) and (max-height: 720px) {
  .signin-page {
    place-items: start center;
  }
}
</style>
