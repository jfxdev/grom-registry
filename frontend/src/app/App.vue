<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { getDeployment } from '@/shared/api/deployment'
import type { Deployment } from '@/shared/api/models'
import { GromBrand } from '@/shared/components/brand'
import { Button } from '@/shared/components/ui/button'
import { ROUTES } from '@/shared/constants'
import {
  Boxes,
  Cable,
  CircleAlert,
  ChevronDown,
  ChevronRight,
  LogOut,
  Menu,
  PanelLeft,
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
  { label: 'Integrations', to: ROUTES.integrations, icon: Cable, adminOnly: false },
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

function handleKeydown(event: globalThis.KeyboardEvent) {
  if (event.key === 'Escape') {
    closeMenu()
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
  void loadDeployment()
})
onBeforeUnmount(() => window.removeEventListener('keydown', handleKeydown))
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

      <div class="sidebar-footer">
        <p class="nav-section-label">Account</p>
        <RouterLink :to="ROUTES.profile" class="user-chip account-link" @click="closeMenu">
          <div class="avatar">{{ session.user?.username.slice(0, 2).toUpperCase() }}</div>
          <div class="min-w-0">
            <p class="truncate text-sm font-medium">{{ session.user?.username }}</p>
            <p class="truncate text-xs text-muted-foreground">{{ session.user?.email }}</p>
          </div>
          <ChevronRight class="ml-auto text-muted-foreground" :size="14" />
        </RouterLink>
        <Button variant="ghost" class="mt-2 w-full justify-start text-muted-foreground" @click="signOut">
          <LogOut :size="16" />
          Sign out
        </Button>
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
        <RouterLink :to="ROUTES.profile" class="header-account">
          <span class="header-account-copy">
            <strong>{{ session.user?.username }}</strong>
            <small>{{ session.user?.systemAdmin ? 'Administrator' : 'Member' }}</small>
          </span>
          <span class="avatar">{{ session.user?.username.slice(0, 2).toUpperCase() }}</span>
          <ChevronDown :size="14" />
        </RouterLink>
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

.account-link {
  color: inherit;
  text-decoration: none;
}

.account-link:hover {
  border-color: var(--border-strong);
  background: var(--surface-raised);
}

.header-account {
  color: inherit;
  text-decoration: none;
}
</style>
