<script setup lang="ts">
import { useSessionStore } from '@/modules/auth/store/session'
import { APIError } from '@/shared/api/client'
import type { AccountedStorageUsage } from '@/shared/api/models'
import { ActionButton, Button, CancelButton } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { PaginationControls } from '@/shared/components/ui/pagination'
import { ROUTES } from '@/shared/constants'
import { pageItems, useCursorPagination } from '@/shared/lib/pagination'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { ArrowUpRight, Box, Plus, Search, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { createProject, listProjects, projectKeys } from '../api/projects'

const queryClient = useQueryClient()
const session = useSessionStore()
const modalOpen = ref(false)
const name = ref('')
const slug = ref('')
const slugEdited = ref(false)
const error = ref('')
const searchQuery = ref('')
const pagination = useCursorPagination()
const projects = useQuery({ queryKey: computed(() => [...projectKeys.all, pagination.cursor.value]), queryFn: () => listProjects(pagination.cursor.value) })
const projectCount = computed(() => pageItems(projects.data.value).length)
const filteredProjects = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  if (!query) return pageItems(projects.data.value)

  return pageItems(projects.data.value).filter((project) =>
    project.name.toLocaleLowerCase().includes(query) || project.slug.toLocaleLowerCase().includes(query),
  )
})
const create = useMutation({
  mutationFn: createProject,
  onMutate: () => {
    error.value = ''
  },
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.all })
    modalOpen.value = false
    name.value = ''
    slug.value = ''
    slugEdited.value = false
    error.value = ''
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create project'
  },
})

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

watch(name, (value) => {
  if (!slugEdited.value) slug.value = slugify(value)
})

function onSlugInput(event: Event) {
  slugEdited.value = (event.target as HTMLInputElement).value.trim() !== ''
}

function onSlugBlur() {
  slug.value = slugify(slug.value)
}

function openCreateModal() {
  error.value = ''
  name.value = ''
  slug.value = ''
  slugEdited.value = false
  modalOpen.value = true
}

function submitCreate() {
  error.value = ''
  create.mutate({ name: name.value, slug: slug.value })
}

function accountedUsageLabel(usage: AccountedStorageUsage | null | undefined) {
  if (!usage) return 'Accounting pending'
  if (usage.status === 'pending') return 'Accounting pending'
  if (usage.status === 'stale') return `${formatBytes(usage.accountedBytes)} (stale)`
  if (usage.status === 'unavailable') return 'Accounting unavailable'
  return formatBytes(usage.accountedBytes)
}

function formatBytes(bytes: number | null | undefined) {
  if (bytes === null || bytes === undefined) return '—'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / (1024 ** index)).toLocaleString(undefined, { maximumFractionDigits: index === 0 ? 0 : 2 })} ${units[index]}`
}
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <h1 class="page-title">Projects</h1>
        <p class="page-description">Manage registry namespaces, repositories and project access.</p>
      </div>
      <ActionButton v-if="session.user?.systemAdmin" @click="openCreateModal"><Plus :size="17" /> New project</ActionButton>
    </header>

    <section class="projects-panel">
      <div class="panel-heading">
        <div>
          <h2>All projects</h2>
          <p>Each project is the first segment of an OCI image path.</p>
        </div>
        <div class="panel-actions">
          <div class="project-search">
            <Search :size="16" aria-hidden="true" />
            <Input
              v-model="searchQuery"
              type="search"
              placeholder="Search projects"
              aria-label="Search projects"
            />
          </div>
          <span v-if="!projects.isLoading.value" class="panel-count">
            {{ searchQuery.trim() ? `${filteredProjects.length} of ${projectCount} on this page` : `${projectCount} on this page` }}
          </span>
        </div>
      </div>

      <div v-if="projects.isLoading.value" class="project-list">
        <div v-for="index in 3" :key="index" class="project-row project-row-loading" />
      </div>
      <div v-else-if="!projectCount" class="empty-state project-empty-state">
        <div>
          <Box class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No projects yet</p>
          <p class="mt-1 text-sm">Create the first namespace for your images.</p>
        </div>
      </div>
      <div v-else-if="!filteredProjects.length" class="empty-state project-empty-state">
        <div>
          <Search class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No matching projects</p>
          <p class="mt-1 text-sm">Try a different name or slug.</p>
        </div>
      </div>
      <div v-else class="project-list">
        <RouterLink
          v-for="project in filteredProjects"
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
          <p class="project-created">{{ accountedUsageLabel(project.accountedUsage) }} accounted</p>
          <p class="project-created">Created {{ new Date(project.createdAt).toLocaleDateString() }}</p>
          <ArrowUpRight class="project-open" :size="17" />
        </RouterLink>
      </div>
      <div class="panel-pagination">
        <PaginationControls
          :page="pagination.page.value"
          :has-previous="pagination.hasPrevious.value"
          :has-next="Boolean(projects.data.value?.nextCursor)"
          :disabled="projects.isFetching.value"
          @previous="pagination.previous()"
          @next="pagination.next(projects.data.value?.nextCursor)"
        />
      </div>
    </section>

    <div v-if="modalOpen && session.user?.systemAdmin" class="modal-backdrop" @click.self="modalOpen = false">
      <form class="modal form-stack" @submit.prevent="submitCreate">
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-lg font-semibold">Create project</h2>
            <p class="mt-1 text-sm text-muted-foreground">The slug becomes the first part of every image path.</p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close" @click="modalOpen = false"><X :size="18" /></Button>
        </div>
        <label class="field-label">Name<Input v-model="name" required /></label>
        <label class="field-label">Slug<Input v-model="slug" required placeholder="payments" @input="onSlugInput" @blur="onSlugBlur" /></label>
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

.panel-actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: 0.65rem;
}

.project-search {
  position: relative;
  width: min(18rem, 34vw);
}

.project-search > svg {
  position: absolute;
  top: 50%;
  left: 0.8rem;
  z-index: 2;
  color: var(--muted-foreground);
  pointer-events: none;
  transform: translateY(-50%);
}

.project-search :deep(.grom-input) {
  min-height: 2.35rem;
  padding-left: 2.35rem;
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
  grid-template-columns: minmax(12rem, 1fr) 8rem auto 11rem auto;
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

.panel-pagination {
  border-top: 1px solid var(--border);
}

@keyframes loading {
  to {
    background-position: -200% 0;
  }
}

@media (max-width: 900px) {
  .project-row {
    grid-template-columns: minmax(10rem, 1fr) auto auto;
  }

  .project-created {
    display: none;
  }
}

@media (max-width: 560px) {
  .panel-heading {
    align-items: stretch;
    flex-direction: column;
  }

  .panel-actions {
    width: 100%;
  }

  .project-search {
    width: 100%;
  }

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
