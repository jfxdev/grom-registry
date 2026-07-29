<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { APIError } from '@/shared/api/client'
import { ActionButton, Button, CancelButton } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { Input } from '@/shared/components/ui/input'
import { ROUTES } from '@/shared/constants'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { ArrowUpRight, Box, Boxes, KeyRound, Plus, ShieldCheck, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { createProject, listProjects, projectKeys } from '../api/projects'

const queryClient = useQueryClient()
const session = useSessionStore()
const modalOpen = ref(false)
const name = ref('')
const slug = ref('')
const error = ref('')
const projects = useQuery({ queryKey: projectKeys.all, queryFn: listProjects })
const projectCount = computed(() => projects.data.value?.length ?? 0)
const create = useMutation({
  mutationFn: createProject,
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.all })
    modalOpen.value = false
    name.value = ''
    slug.value = ''
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create project'
  },
})

function syncSlug() {
  slug.value = name.value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <h1 class="page-title">Projects</h1>
        <p class="page-description">Manage registry namespaces, repositories and project access.</p>
      </div>
      <ActionButton v-if="session.user?.systemAdmin" @click="modalOpen = true"><Plus :size="17" /> New project</ActionButton>
    </header>

    <section class="overview-grid" aria-label="Registry overview">
      <Card class="overview-card">
        <div class="overview-card-heading">
          <span>Projects</span>
          <Boxes :size="16" />
        </div>
        <p class="overview-value">{{ projects.isLoading.value ? '—' : projectCount }}</p>
        <p class="overview-detail">Immutable registry namespaces</p>
      </Card>
      <Card class="overview-card">
        <div class="overview-card-heading">
          <span>Authorization</span>
          <ShieldCheck :size="16" />
        </div>
        <p class="overview-value overview-value-label">Project scoped</p>
        <p class="overview-detail">Reader, writer and admin roles</p>
      </Card>
      <Card class="overview-card">
        <div class="overview-card-heading">
          <span>Registry access</span>
          <KeyRound :size="16" />
        </div>
        <p class="overview-value overview-value-label">Service accounts</p>
        <p class="overview-detail">Reveal-once access keys</p>
      </Card>
    </section>

    <section class="projects-panel">
      <div class="panel-heading">
        <div>
          <h2>All projects</h2>
          <p>Each project is the first segment of an OCI image path.</p>
        </div>
        <span v-if="!projects.isLoading.value" class="panel-count">{{ projectCount }} total</span>
      </div>

      <div v-if="projects.isLoading.value" class="project-list">
        <div v-for="index in 3" :key="index" class="project-row project-row-loading" />
      </div>
      <div v-else-if="!projects.data.value?.length" class="empty-state project-empty-state">
        <div>
          <Box class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No projects yet</p>
          <p class="mt-1 text-sm">Create the first namespace for your images.</p>
        </div>
      </div>
      <div v-else class="project-list">
        <RouterLink
          v-for="project in projects.data.value"
          :key="project.id"
          :to="`${ROUTES.projects}/${project.slug}`"
          class="project-row group"
        >
          <div class="project-identity">
            <div class="project-icon"><Box :size="17" /></div>
            <div class="min-w-0">
              <h3>{{ project.name }}</h3>
              <p>{{ project.slug }}/</p>
            </div>
          </div>
          <div class="project-meta">
            <span class="status-dot" />
            Active
          </div>
          <p class="project-created">Created {{ new Date(project.createdAt).toLocaleDateString() }}</p>
          <ArrowUpRight class="project-open" :size="17" />
        </RouterLink>
      </div>
    </section>

    <div v-if="modalOpen && session.user?.systemAdmin" class="modal-backdrop" @click.self="modalOpen = false">
      <form class="modal form-stack" @submit.prevent="create.mutate({ name, slug })">
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-lg font-semibold">Create project</h2>
            <p class="mt-1 text-sm text-muted-foreground">The slug becomes the first part of every image path.</p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close" @click="modalOpen = false"><X :size="18" /></Button>
        </div>
        <label class="field-label">Name<Input v-model="name" required @blur="!slug && syncSlug()" /></label>
        <label class="field-label">Slug<Input v-model="slug" required placeholder="payments" /></label>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="flex justify-end gap-2">
          <CancelButton @click="modalOpen = false" />
          <ActionButton type="submit" :disabled="create.isPending.value">Create project</ActionButton>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.overview-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.85rem;
  margin-bottom: 1rem;
}

.overview-card {
  min-height: 8.9rem;
  padding: 1.1rem 1.2rem;
}

.overview-card-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--muted-foreground);
  font-size: 0.75rem;
  font-weight: 580;
}

.overview-value {
  margin: 1.15rem 0 0;
  color: var(--foreground);
  font-size: 1.8rem;
  font-weight: 700;
  letter-spacing: -0.04em;
}

.overview-value-label {
  font-size: 1.35rem;
}

.overview-detail {
  margin: 0.25rem 0 0;
  color: var(--muted-foreground);
  font-size: 0.72rem;
}

.projects-panel {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 0.7rem;
  background: var(--surface);
}

.panel-heading {
  display: flex;
  min-height: 4.65rem;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid var(--border);
  padding: 0.9rem 1.2rem;
}

.panel-heading h2 {
  margin: 0;
  font-size: 0.92rem;
  font-weight: 650;
}

.panel-heading p {
  margin: 0.3rem 0 0;
  color: var(--muted-foreground);
  font-size: 0.75rem;
}

.panel-count {
  border: 1px solid var(--border);
  border-radius: 0.45rem;
  padding: 0.3rem 0.5rem;
  color: var(--muted-foreground);
  font-size: 0.7rem;
}

.project-list {
  display: grid;
}

.project-row {
  display: grid;
  grid-template-columns: minmax(12rem, 1fr) 8rem 11rem auto;
  align-items: center;
  gap: 1rem;
  min-height: 4.65rem;
  border-top: 1px solid var(--border);
  padding: 0.75rem 1.2rem;
  color: inherit;
  text-decoration: none;
  transition: background var(--motion-standard) ease;
}

.project-row:first-child {
  border-top: 0;
}

.project-row:hover {
  background: rgba(255, 255, 255, 0.025);
}

.project-row-loading {
  min-height: 4.65rem;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.025), transparent);
  background-size: 200% 100%;
  animation: loading 1.2s ease-in-out infinite;
}

.project-identity {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 0.8rem;
}

.project-icon {
  display: grid;
  width: 2.25rem;
  height: 2.25rem;
  flex: none;
  place-items: center;
  border-radius: 0.55rem;
  border: 1px solid rgba(145, 173, 36, 0.12);
  background: rgba(145, 173, 36, 0.08);
  color: #c9df6c;
}

.project-identity h3 {
  overflow: hidden;
  margin: 0;
  color: var(--foreground);
  font-size: 0.82rem;
  font-weight: 620;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-identity p,
.project-created {
  margin: 0.25rem 0 0;
  color: var(--muted-foreground);
  font-size: 0.7rem;
}

.project-identity p {
  overflow: hidden;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-meta {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  color: var(--muted-foreground);
  font-size: 0.72rem;
}

.status-dot {
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 999px;
  background: var(--accent);
  box-shadow: 0 0 0 3px rgba(145, 173, 36, 0.09);
}

.project-created {
  margin: 0;
}

.project-open {
  color: var(--muted-foreground);
  transition: color var(--motion-standard) ease;
}

.project-row:hover .project-open {
  color: var(--accent);
}

.project-empty-state {
  min-height: 18rem;
  border: 0;
  border-radius: 0;
}

@keyframes loading {
  to {
    background-position: -200% 0;
  }
}

@media (max-width: 900px) {
  .overview-grid {
    grid-template-columns: 1fr;
  }

  .overview-card {
    min-height: 0;
  }

  .project-row {
    grid-template-columns: minmax(10rem, 1fr) auto;
  }

  .project-created {
    display: none;
  }
}

@media (max-width: 560px) {
  .project-meta {
    display: none;
  }

  .project-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .project-row-loading {
    animation: none;
  }
}
</style>
