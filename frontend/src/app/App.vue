<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { getDeployment } from '@/shared/api/deployment'
import type { Deployment } from '@/shared/api/models'
import { GromBrand } from '@/shared/components/brand'
import { Button } from '@/shared/components/ui/button'
import { ROUTES } from '@/shared/constants'
import {
  Boxes,
  CircleAlert,
  ChevronDown,
  ChevronRight,
  LogOut,
  Menu,
  PanelLeft,
  UserRound,
  ShieldCheck,
  DatabaseBackup,
  Users,
  X,
} from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const menuOpen = ref(false)
const accountMenuOpen = ref(false)
const deployment = ref<Deployment | null>(null)
const insecureHTTP = computed(() => deployment.value?.insecureHttp === true)
const isPublic = computed(() => route.meta.public === true)
const currentSection = computed(() => {
  if (route.path.startsWith(`${ROUTES.projects}/`)) {
    return 'Project'
  }

  return visibleNavigation.value.find((item) => item.to === route.path)?.label ?? 'Overview'
})

const navigation = [
  { label: 'Projects', to: ROUTES.projects, icon: Boxes, adminOnly: false },
  { label: 'Users', to: ROUTES.users, icon: Users, adminOnly: true },
  { label: 'Service accounts', to: ROUTES.serviceAccounts, icon: ShieldCheck, adminOnly: false },
  { label: 'Backup & recovery', to: ROUTES.backups, icon: DatabaseBackup, adminOnly: true },
]
const visibleNavigation = computed(() => navigation.filter((item) => !item.adminOnly || session.user?.systemAdmin))

async function signOut() {
  await session.signOut()
  await router.push('/signin')
}

function closeMenu() {
  menuOpen.value = false
}

function closeAccountMenu() {
  accountMenuOpen.value = false
}

function toggleAccountMenu() {
  accountMenuOpen.value = !accountMenuOpen.value
}

function handleKeydown(event: globalThis.KeyboardEvent) {
  if (event.key === 'Escape') {
    closeMenu()
    closeAccountMenu()
  }
}

function handlePointerDown(event: globalThis.PointerEvent) {
  const target = event.target
  if (target instanceof globalThis.Element && !target.closest('.account-menu')) {
    closeAccountMenu()
  }
}

async function loadDeployment() {
  try {
    deployment.value = await getDeployment()
  } catch {
    deployment.value = null
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('pointerdown', handlePointerDown)
  void loadDeployment()
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('pointerdown', handlePointerDown)
})
</script>

<template>
  <template v-if="isPublic">
    <div v-if="insecureHTTP" class="deployment-warning" role="alert">
      <CircleAlert :size="17" aria-hidden="true" />
      <span><strong>Insecure HTTP deployment.</strong> Use this permissive profile only on a trusted private network.</span>
    </div>
    <RouterView />
  </template>
  <div v-else class="app-shell min-h-screen bg-background text-foreground">
    <header class="mobile-header">
      <RouterLink :to="ROUTES.projects" class="shell-brand-link">
        <GromBrand compact :crest-size="34" />
      </RouterLink>
      <div class="mobile-header-actions">
        <div class="account-menu account-menu-mobile">
          <Button
            variant="ghost"
            size="icon"
            aria-controls="mobile-account-menu"
            :aria-expanded="accountMenuOpen"
            aria-haspopup="true"
            aria-label="Open account menu"
            @click="toggleAccountMenu"
          >
            <span class="avatar">{{ session.user?.username.slice(0, 2).toUpperCase() }}</span>
          </Button>
          <div v-if="accountMenuOpen" id="mobile-account-menu" class="account-menu-panel" aria-label="Account menu">
            <RouterLink :to="ROUTES.profile" class="account-menu-item" @click="closeAccountMenu">
              <UserRound :size="16" />
              Profile
            </RouterLink>
            <button type="button" class="account-menu-item" @click="signOut">
              <LogOut :size="16" />
              Sign out
            </button>
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          aria-controls="app-navigation"
          :aria-expanded="menuOpen"
          :aria-label="menuOpen ? 'Close navigation' : 'Open navigation'"
          @click="menuOpen = !menuOpen"
        >
          <X v-if="menuOpen" :size="20" />
          <Menu v-else :size="20" />
        </Button>
      </div>
    </header>

    <button
      v-if="menuOpen"
      type="button"
      class="sidebar-backdrop"
      aria-label="Close navigation"
      @click="closeMenu"
    />

    <aside id="app-navigation" class="sidebar" :class="{ 'sidebar-open': menuOpen }">
      <div class="min-h-0">
        <div class="sidebar-brand">
          <RouterLink :to="ROUTES.projects" class="shell-brand-link">
            <GromBrand compact :crest-size="34" />
          </RouterLink>
          <span class="environment-badge" :class="{ 'environment-badge-warning': insecureHTTP }">
            {{ insecureHTTP ? 'Insecure HTTP' : 'Registry' }}
          </span>
        </div>

        <div class="sidebar-primary-action">
          <Button class="w-full justify-start" size="sm" @click="router.push(ROUTES.projects)">
            <Boxes :size="15" />
            Browse projects
          </Button>
        </div>

        <nav class="sidebar-navigation">
          <p class="nav-section-label">Platform</p>
          <RouterLink
            v-for="item in visibleNavigation"
            :key="item.to"
            :to="item.to"
            class="nav-link"
            :class="{ active: route.path === item.to || (item.to === ROUTES.projects && route.path.startsWith(`${ROUTES.projects}/`)) }"
            @click="closeMenu"
          >
            <component :is="item.icon" :size="16" />
            <span>{{ item.label }}</span>
            <ChevronRight class="ml-auto nav-chevron" :size="14" />
          </RouterLink>
        </nav>
      </div>
    </aside>

    <div class="app-workspace">
      <header class="desktop-header">
        <div class="header-context">
          <PanelLeft :size="17" />
          <span class="header-divider" />
          <span class="header-product">Grom Registry</span>
          <ChevronRight :size="13" />
          <span class="header-current">{{ currentSection }}</span>
        </div>
        <div class="account-menu">
          <button
            type="button"
            class="header-account"
            aria-controls="desktop-account-menu"
            :aria-expanded="accountMenuOpen"
            aria-haspopup="true"
            @click="toggleAccountMenu"
          >
            <span class="header-account-copy">
              <strong>{{ session.user?.username }}</strong>
              <small>{{ session.user?.systemAdmin ? 'Administrator' : session.user?.systemViewer ? 'Viewer' : 'Member' }}</small>
            </span>
            <span class="avatar">{{ session.user?.username.slice(0, 2).toUpperCase() }}</span>
            <ChevronDown :size="14" :class="{ 'account-menu-chevron-open': accountMenuOpen }" />
          </button>
          <div v-if="accountMenuOpen" id="desktop-account-menu" class="account-menu-panel" aria-label="Account menu">
            <RouterLink :to="ROUTES.profile" class="account-menu-item" @click="closeAccountMenu">
              <UserRound :size="16" />
              Profile
            </RouterLink>
            <button type="button" class="account-menu-item" @click="signOut">
              <LogOut :size="16" />
              Sign out
            </button>
          </div>
        </div>
      </header>

      <div v-if="insecureHTTP" class="deployment-warning" role="alert">
        <CircleAlert :size="17" aria-hidden="true" />
        <span><strong>Insecure HTTP deployment.</strong> Use this permissive profile only on a trusted private network.</span>
      </div>

      <main class="app-main">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<style scoped>
.shell-brand-link {
  display: inline-flex;
  color: inherit;
  text-decoration: none;
}

.header-account {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.header-account:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 3px;
}

.account-menu {
  position: relative;
}

.account-menu-panel {
  position: absolute;
  top: calc(100% + 0.55rem);
  right: 0;
  z-index: 50;
  width: 11rem;
  overflow: hidden;
  border: 1px solid var(--border-strong);
  border-radius: 0.65rem;
  background: var(--surface-raised);
  box-shadow: 0 0.75rem 1.5rem rgba(0, 0, 0, 0.3);
  padding: 0.3rem;
}

.account-menu-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 0.6rem;
  border: 0;
  border-radius: 0.4rem;
  background: transparent;
  padding: 0.65rem 0.7rem;
  color: var(--foreground);
  font: inherit;
  font-size: 0.8rem;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
}

.account-menu-item:hover,
.account-menu-item:focus-visible {
  background: var(--muted);
  outline: none;
}

.account-menu-chevron-open {
  transform: rotate(180deg);
}

.mobile-header-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.account-menu-mobile .avatar {
  width: 1.9rem;
  height: 1.9rem;
}
</style>
