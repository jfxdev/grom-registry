<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import { Badge } from '@/shared/components/ui/badge'
import { Input } from '@/shared/components/ui/input'
import { PaginationControls } from '@/shared/components/ui/pagination'
import { pageItems, useCursorPagination } from '@/shared/lib/pagination'
import { useQuery } from '@tanstack/vue-query'
import { Box, Search } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { repositorySearchKeys, searchRepositories } from '../api/repositories'

const router = useRouter()
const pagination = useCursorPagination()
const searchQuery = ref('')
const results = useQuery({
  queryKey: computed(() => repositorySearchKeys.list(searchQuery.value.trim(), pagination.cursor.value)),
  queryFn: () => searchRepositories(searchQuery.value.trim(), pagination.cursor.value),
  enabled: computed(() => searchQuery.value.trim() !== ''),
})
const items = computed(() => pageItems(results.data.value))
const resultsError = computed(() =>
  results.error.value instanceof APIError ? results.error.value.message : 'Could not search repositories. Please try again.',
)

watch(searchQuery, () => pagination.reset())

function openRepository(item: { projectSlug: string, id: string }) {
  void router.push({ name: 'repository-detail', params: { project: item.projectSlug, repositoryId: item.id } })
}
</script>

<template>
  <div class="page-shell">
    <header class="page-header">
      <div>
        <p class="eyebrow">Installation-wide</p>
        <h1 class="page-title">Repository search</h1>
        <p class="page-description">Find a repository by name or description across every project.</p>
      </div>
    </header>

    <div class="table-shell">
      <div class="list-toolbar">
        <div>
          <h2>Repositories</h2>
          <p>Results are scoped to whatever you type below; nothing loads until you search.</p>
        </div>
        <div class="list-toolbar-actions">
          <label class="list-search">
            <Search :size="16" aria-hidden="true" />
            <Input v-model="searchQuery" type="search" placeholder="Search repositories" aria-label="Search repositories" />
          </label>
        </div>
      </div>

      <div v-if="results.isLoadingError.value" class="empty-state list-empty-state" role="alert">
        <div>
          <p class="font-medium text-foreground">Could not load results</p>
          <p class="mt-2 text-sm text-muted-foreground">{{ resultsError }}</p>
        </div>
      </div>
      <div v-else-if="results.isLoading.value" class="empty-state list-empty-state" role="status">
        <p class="text-sm text-muted-foreground">Searching…</p>
      </div>
      <div v-else-if="!items.length && searchQuery.trim()" class="empty-state list-empty-state">
        <div>
          <Search class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No matching repositories</p>
          <p class="mt-1 text-sm">Try a different name or description.</p>
        </div>
      </div>
      <div v-else-if="!items.length" class="empty-state list-empty-state">
        <div>
          <Box class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">Type to search</p>
          <p class="mt-1 text-sm">Search spans every project you administer.</p>
        </div>
      </div>
      <template v-else>
        <div class="table-shell">
          <button v-for="item in items" :key="item.id" class="data-row w-full text-left" @click="openRepository(item)">
            <div class="flex items-center gap-3">
              <div class="avatar"><Box :size="15" /></div>
              <div>
                <p class="text-sm font-semibold">{{ item.name }}</p>
                <p class="mt-1 font-mono text-xs text-muted-foreground">{{ item.projectSlug }}/{{ item.name }}</p>
              </div>
            </div>
            <div class="flex items-center justify-end gap-2">
              <Badge tone="neutral">{{ item.projectName }}</Badge>
              <p class="text-xs text-muted-foreground">{{ item.status }}</p>
            </div>
          </button>
        </div>
      </template>
      <div class="list-pagination">
        <PaginationControls
          :page="pagination.page.value"
          :has-previous="pagination.hasPrevious.value"
          :has-next="Boolean(results.data.value?.nextCursor)"
          :disabled="results.isFetching.value"
          @previous="pagination.previous()"
          @next="pagination.next(results.data.value?.nextCursor)"
        />
      </div>
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
  display: block;
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

.list-empty-state {
  min-height: 16rem;
  border: 0;
  border-radius: 0;
}

.list-pagination {
  border-top: 1px solid var(--border);
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
