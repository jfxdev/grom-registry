<script setup lang="ts">
import { listServiceAccounts, serviceAccountKeys } from '@/modules/service-accounts/api/serviceAccounts'
import { useSessionStore } from '@/modules/auth/store/session'
import { listUsers, userKeys } from '@/modules/users/api/users'
import {
  createLifecyclePreview,
  executeLifecycle,
  listLifecycleRuns,
  registryKeys,
} from '@/modules/registry'
import { APIError } from '@/shared/api/client'
import type {
  ArtifactDeletionPreview,
  LifecyclePreview,
  LifecycleRun,
  PrincipalKind,
  ProjectRole,
  Repository,
  RepositoryPolicySet,
} from '@/shared/api/models'
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { Dialog } from '@/shared/components/ui/dialog'
import { ROUTES } from '@/shared/constants'
import { writeClipboardText } from '@/shared/lib/clipboard'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { AlertTriangle, Box, Check, ChevronLeft, Clipboard, PackageOpen, Plus, RefreshCw, Settings2, Trash2, Users, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  deleteArtifact,
  deleteProject,
  getProject,
  listArtifactDeletions,
  listMembers,
  listRepositories,
  listTags,
  projectKeys,
  previewArtifactDeletion,
  setMember,
} from '../api/projects'
import RepositoryCreateModal from '../components/RepositoryCreateModal.vue'
import RepositoryPolicyModal from '../components/RepositoryPolicyModal.vue'

const route = useRoute()
const router = useRouter()
const queryClient = useQueryClient()
const session = useSessionStore()
const slug = computed(() => String(route.params.project))
const registryHost = window.location.host
const activeTab = ref<'repositories' | 'members'>('repositories')
const selectedRepository = ref<Repository | null>(null)
const repositoryModal = ref(false)
const policyModal = ref(false)
const memberModal = ref(false)
const memberId = ref('')
const memberKind = ref<PrincipalKind>('service_account')
const memberRole = ref<ProjectRole>('reader')
const memberError = ref('')
const copied = ref('')
const copyError = ref('')
const deletionPreview = ref<ArtifactDeletionPreview | null>(null)
const deletionReference = ref('')
const deletionReason = ref('')
const deletionError = ref('')
const lifecyclePreview = ref<LifecyclePreview | null>(null)
const lifecycleRun = ref<LifecycleRun | null>(null)
const lifecycleReason = ref('')
const lifecycleError = ref('')
const projectDeletionOpen = ref(false)
const projectDeletionError = ref('')

const project = useQuery({ queryKey: computed(() => projectKeys.detail(slug.value)), queryFn: () => getProject(slug.value) })
const repositories = useQuery({ queryKey: computed(() => projectKeys.repositories(slug.value)), queryFn: () => listRepositories(slug.value) })
const members = useQuery({ queryKey: computed(() => projectKeys.members(slug.value)), queryFn: () => listMembers(slug.value) })
const accounts = useQuery({ queryKey: serviceAccountKeys.all, queryFn: listServiceAccounts })
const users = useQuery({ queryKey: userKeys.all, queryFn: listUsers, enabled: computed(() => session.user?.systemAdmin === true) })
const canManage = computed(() =>
  session.user?.systemAdmin === true ||
  members.data.value?.some((member) =>
    member.principalKind === 'user' &&
    member.principalId === session.user?.id &&
    member.role === 'admin',
  ) === true,
)
const tags = useQuery({
  queryKey: computed(() => projectKeys.tags(slug.value, selectedRepository.value?.name ?? '')),
  queryFn: () => listTags(slug.value, selectedRepository.value!.name),
  enabled: computed(() => selectedRepository.value !== null),
})
const artifactDeletionHistory = useQuery({
  queryKey: computed(() => projectKeys.artifactDeletions(slug.value, selectedRepository.value?.name ?? '')),
  queryFn: () => listArtifactDeletions(slug.value, selectedRepository.value!.name),
  enabled: computed(() => canManage.value && selectedRepository.value !== null),
})
const lifecycleHistory = useQuery({
  queryKey: computed(() => registryKeys.lifecycleRuns(slug.value, selectedRepository.value?.name ?? '')),
  queryFn: () => listLifecycleRuns(slug.value, selectedRepository.value!.name),
  enabled: computed(() => canManage.value && selectedRepository.value !== null),
})

const removeProject = useMutation({
  mutationFn: () => deleteProject(slug.value),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.all })
    await router.push(ROUTES.projects)
  },
  onError: (caught) => {
    projectDeletionError.value = caught instanceof APIError ? caught.message : 'Could not delete this project'
  },
})

const addMember = useMutation({
  mutationFn: () => setMember(slug.value, memberKind.value, memberId.value, memberRole.value),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.members(slug.value) })
    memberError.value = ''
    memberModal.value = false
  },
  onError: (caught) => {
    memberError.value = caught instanceof APIError ? caught.message : 'Could not add this project member'
  },
})

const previewDeletion = useMutation({
  mutationFn: (tag: string) => previewArtifactDeletion(slug.value, {
    repository: selectedRepository.value!.name,
    reference: tag,
    reason: '',
  }),
  onSuccess: (preview, tag) => {
    deletionReference.value = tag
    deletionPreview.value = preview
    deletionReason.value = ''
    deletionError.value = ''
  },
  onError: (caught) => {
    deletionError.value = caught instanceof APIError ? caught.message : 'Could not review this deletion'
  },
})

const confirmDeletion = useMutation({
  mutationFn: () => deleteArtifact(slug.value, {
    repository: deletionPreview.value!.repository,
    reference: deletionReference.value,
    reason: deletionReason.value,
    expectedDigest: deletionPreview.value!.digest,
    expectedTags: deletionPreview.value!.affectedTags,
  }),
  onSuccess: async (deletion) => {
    const repository = deletion.repository
    await queryClient.invalidateQueries({ queryKey: projectKeys.tags(slug.value, repository) })
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    await queryClient.invalidateQueries({
      queryKey: projectKeys.artifactDeletions(slug.value, repository),
    })
    deletionPreview.value = null
  },
  onError: (caught) => {
    deletionError.value = caught instanceof APIError ? caught.message : 'Could not delete this artifact'
  },
})

const reviewLifecycle = useMutation({
  mutationFn: () => createLifecyclePreview(slug.value, selectedRepository.value!.name),
  onSuccess: (preview) => {
    lifecyclePreview.value = preview
    lifecycleRun.value = null
    lifecycleReason.value = ''
    lifecycleError.value = ''
  },
  onError: (caught) => {
    lifecycleError.value = caught instanceof APIError ? caught.message : 'Could not create the lifecycle review'
  },
})

const runLifecycle = useMutation({
  mutationFn: () => executeLifecycle(slug.value, lifecyclePreview.value!.id, lifecycleReason.value),
  onSuccess: async (run) => {
    const repository = run.repository
    lifecycleRun.value = run
    lifecycleError.value = ''
    await queryClient.invalidateQueries({ queryKey: projectKeys.tags(slug.value, repository) })
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    await queryClient.invalidateQueries({ queryKey: registryKeys.inventory(slug.value, repository) })
    await queryClient.invalidateQueries({ queryKey: registryKeys.lifecycleRuns(slug.value, repository) })
  },
  onError: (caught) => {
    lifecycleError.value = caught instanceof APIError ? caught.message : 'Could not execute lifecycle'
  },
})

const availableAccounts = computed(() => {
  const assigned = new Set(members.data.value?.filter((member) => member.principalKind === 'service_account').map((member) => member.principalId))
  return accounts.data.value?.filter((account) => !assigned.has(account.id)) ?? []
})
const availableUsers = computed(() => {
  const assigned = new Set(members.data.value?.filter((member) => member.principalKind === 'user').map((member) => member.principalId))
  return users.data.value?.filter((user) => !assigned.has(user.id)) ?? []
})
const availablePrincipals = computed(() => memberKind.value === 'user' ? availableUsers.value : availableAccounts.value)
async function policiesSaved(policySet: RepositoryPolicySet) {
  if (selectedRepository.value?.id === policySet.repositoryId) {
    selectedRepository.value = {
      ...selectedRepository.value,
      policyVersion: policySet.version,
      policies: policySet.policies,
    }
  }
  policyModal.value = false
  await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
}

async function copyCommand(repository: string, tag = 'latest') {
  const command = `docker pull ${window.location.host}/${slug.value}/${repository}:${tag}`
  const result = await writeClipboardText(command)
  if (result !== 'copied') {
    copyError.value = 'Could not copy the pull command. Select and copy it manually.'
    return
  }
  copyError.value = ''
  copied.value = repository + tag
  window.setTimeout(() => { copied.value = '' }, 1200)
}

function openMemberModal() {
  if (!canManage.value) return
  memberKind.value = 'service_account'
  memberId.value = availableAccounts.value[0]?.id ?? ''
  memberError.value = ''
  memberModal.value = true
}

function closeMemberModal() {
  memberModal.value = false
  memberError.value = ''
}

function submitMember() {
  if (!canManage.value || !memberId.value) return
  addMember.mutate()
}

function changeMemberKind() {
  memberId.value = availablePrincipals.value[0]?.id ?? ''
}

function profileLabel(profile: Repository['profile']) {
  return profile.replaceAll('_', ' ')
}
</script>

<template>
  <div class="page-shell">
    <RouterLink :to="ROUTES.projects" class="mb-5 inline-flex items-center gap-1 text-xs text-muted-foreground no-underline hover:text-foreground">
      <ChevronLeft :size="15" /> Projects
    </RouterLink>
    <header class="page-header">
      <div>
        <p class="eyebrow">Project namespace</p>
        <h1 class="page-title">{{ project.data.value?.name ?? slug }}</h1>
        <p class="page-description font-mono">{{ registryHost }}/{{ slug }}/</p>
      </div>
      <div class="flex items-center gap-2">
        <Badge tone="success">Active</Badge>
        <Button v-if="session.user?.systemAdmin" variant="danger" size="sm" @click="projectDeletionOpen = true">
          <Trash2 :size="14" /> Delete project
        </Button>
      </div>
    </header>

    <div class="tab-list">
      <button :class="{ active: activeTab === 'repositories' }" @click="activeTab = 'repositories'"><PackageOpen :size="16" /> Repositories</button>
      <button :class="{ active: activeTab === 'members' }" @click="activeTab = 'members'"><Users :size="16" /> Members</button>
    </div>

    <section v-if="activeTab === 'repositories'">
      <div v-if="canManage" class="mt-5 flex justify-end">
        <Button size="sm" @click="repositoryModal = true"><Plus :size="15" /> New repository</Button>
      </div>
      <div v-if="!repositories.data.value?.length" class="empty-state mt-5">
        <div>
          <Box class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No repositories yet</p>
          <p class="mt-2 text-sm">A Writer or Admin can create one automatically with the first push.</p>
        </div>
      </div>
      <div v-else class="table-shell mt-5">
        <button v-for="repository in repositories.data.value" :key="repository.id" class="data-row w-full text-left" @click="selectedRepository = repository">
          <div class="flex items-center gap-3">
            <div class="avatar"><Box :size="15" /></div>
            <div><p class="text-sm font-semibold">{{ repository.name }}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{{ slug }}/{{ repository.name }}</p></div>
          </div>
          <div class="flex items-center justify-end gap-2">
            <Badge :tone="repository.profileNeedsReview ? 'danger' : repository.profile === 'unknown' ? 'neutral' : 'success'">
              {{ profileLabel(repository.profile) }}
            </Badge>
            <p class="text-xs text-muted-foreground">
              {{ repository.policies.length }} policies · {{ repository.status }}
              <template v-if="repository.creationSource === 'push'"> · created by push</template>
            </p>
          </div>
          <span class="flex items-center gap-2 text-accent"><Settings2 :size="14" /> →</span>
        </button>
      </div>
    </section>

    <section v-else>
      <div v-if="canManage" class="mt-5 flex justify-end"><Button size="sm" @click="openMemberModal"><Plus :size="15" /> Add service account</Button></div>
      <div class="table-shell mt-3">
        <div v-for="member in members.data.value" :key="`${member.principalKind}:${member.principalId}`" class="data-row">
          <div><p class="text-sm font-semibold">{{ member.principalKind.replace('_', ' ') }}</p><p class="mt-1 font-mono text-xs text-muted-foreground">{{ member.principalId }}</p></div>
          <p class="text-xs text-muted-foreground">Assigned {{ new Date(member.createdAt).toLocaleDateString() }}</p>
          <Badge>{{ member.role }}</Badge>
        </div>
      </div>
    </section>

    <Dialog
      v-if="projectDeletionOpen && session.user?.systemAdmin"
      labelled-by="delete-project-title"
      @close="projectDeletionOpen = false"
    >
      <form class="modal form-stack" aria-labelledby="delete-project-title" @submit.prevent="removeProject.mutate()">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="eyebrow">Destructive action</p>
            <h2 id="delete-project-title" class="text-lg font-semibold">Delete project</h2>
          </div>
          <Button variant="ghost" size="icon" type="button" aria-label="Close project deletion" @click="projectDeletionOpen = false">
            <X :size="18" />
          </Button>
        </div>
        <div class="deletion-warning">
          <AlertTriangle :size="18" />
          <p>
            Delete <strong>{{ project.data.value?.name ?? slug }}</strong> and all of its memberships?
            Projects containing repositories cannot be deleted.
          </p>
        </div>
        <p v-if="projectDeletionError" class="error-text">{{ projectDeletionError }}</p>
        <div class="flex justify-end gap-2">
          <Button variant="ghost" type="button" @click="projectDeletionOpen = false">Cancel</Button>
          <Button variant="danger" type="submit" :disabled="removeProject.isPending.value">
            {{ removeProject.isPending.value ? 'Deleting…' : 'Delete project' }}
          </Button>
        </div>
      </form>
    </Dialog>

    <Dialog v-if="selectedRepository" labelled-by="repository-details-title" @close="selectedRepository = null">
      <section class="modal form-stack" aria-labelledby="repository-details-title">
        <div class="flex items-start justify-between">
          <div>
            <p class="eyebrow">Repository</p>
            <h2 id="repository-details-title" class="text-xl font-semibold">{{ selectedRepository.name }}</h2>
            <div class="mt-2 flex items-center gap-2">
              <Badge :tone="selectedRepository.profileNeedsReview ? 'danger' : selectedRepository.profile === 'unknown' ? 'neutral' : 'success'">
                {{ profileLabel(selectedRepository.profile) }}
              </Badge>
              <span class="text-xs text-muted-foreground">
                {{ selectedRepository.profileSource === 'inferred'
                  ? `${selectedRepository.profileConfidence} confidence · inferred from uploaded content`
                  : 'Waiting for the first identifiable upload' }}
              </span>
            </div>
            <p v-if="selectedRepository.profileNeedsReview" class="mt-2 text-xs text-destructive">
              Different primary artifact types were detected in this repository.
            </p>
          </div>
          <div class="flex items-center gap-2">
            <Button
              v-if="canManage"
              variant="outline"
              size="sm"
              @click="policyModal = true"
            >
              <Settings2 :size="14" /> Policies
            </Button>
            <Button
              v-if="canManage"
              variant="outline"
              size="sm"
              :disabled="reviewLifecycle.isPending.value"
              @click="reviewLifecycle.mutate()"
            >
              <RefreshCw :size="14" /> {{ reviewLifecycle.isPending.value ? 'Reviewing…' : 'Review lifecycle' }}
            </Button>
            <Button variant="ghost" size="icon" @click="selectedRepository = null"><X :size="18" /></Button>
          </div>
        </div>
        <div v-if="!tags.data.value?.tags?.length" class="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">No tags available.</div>
        <div v-else class="space-y-2">
          <Card v-for="tag in tags.data.value.tags" :key="tag" class="flex items-center justify-between p-3">
            <code class="text-sm text-accent">{{ tag }}</code>
            <div class="flex gap-1">
              <Button variant="ghost" size="sm" @click="copyCommand(selectedRepository!.name, tag)">
                <Check v-if="copied === selectedRepository.name + tag" :size="14" /><Clipboard v-else :size="14" /> Copy pull
              </Button>
              <Button
                v-if="canManage"
                variant="ghost"
                size="icon"
                :aria-label="`Delete ${tag}`"
                :disabled="previewDeletion.isPending.value"
                @click="previewDeletion.mutate(tag)"
              >
                <Trash2 :size="15" />
              </Button>
            </div>
          </Card>
        </div>
        <p v-if="copyError" class="error-text" role="alert">{{ copyError }}</p>
        <p v-if="deletionError && !deletionPreview" class="error-text">{{ deletionError }}</p>
        <p v-if="lifecycleError && !lifecyclePreview" class="error-text">{{ lifecycleError }}</p>
        <div v-if="canManage && (artifactDeletionHistory.data.value?.length || lifecycleHistory.data.value?.length)" class="operation-history">
          <h3 class="text-sm font-semibold">Deletion history</h3>
          <Card v-for="deletion in artifactDeletionHistory.data.value?.slice(0, 5)" :key="deletion.id" class="p-3">
            <div class="flex items-center justify-between gap-3">
              <code class="break-all text-xs">{{ deletion.digest }}</code>
              <Badge :tone="deletion.status === 'completed' ? 'success' : 'danger'">{{ deletion.status }}</Badge>
            </div>
            <p class="mt-1 text-xs text-muted-foreground">Manual · {{ new Date(deletion.startedAt).toLocaleString() }} · {{ deletion.reason || 'No reason' }}</p>
          </Card>
          <Card v-for="run in lifecycleHistory.data.value?.slice(0, 5)" :key="run.id" class="p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs">Lifecycle · {{ run.items.length }} candidates</span>
              <Badge :tone="run.status === 'completed' ? 'success' : run.status === 'failed' ? 'danger' : 'warning'">{{ run.status }}</Badge>
            </div>
            <p class="mt-1 text-xs text-muted-foreground">{{ new Date(run.startedAt).toLocaleString() }} · {{ run.reason }}</p>
          </Card>
        </div>
      </section>
    </Dialog>

    <RepositoryCreateModal
      v-if="repositoryModal"
      :project="slug"
      @close="repositoryModal = false"
      @created="repositoryModal = false"
    />

    <Dialog v-if="deletionPreview" labelled-by="delete-artifact-title" @close="deletionPreview = null">
      <form class="modal form-stack" aria-labelledby="delete-artifact-title" @submit.prevent="confirmDeletion.mutate()">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="eyebrow">Destructive action</p>
            <h2 id="delete-artifact-title" class="text-lg font-semibold">Delete artifact</h2>
          </div>
          <Button variant="ghost" size="icon" type="button" aria-label="Close deletion" @click="deletionPreview = null">
            <X :size="18" />
          </Button>
        </div>
        <div class="deletion-warning">
          <AlertTriangle :size="18" />
          <p>This removes the manifest and every tag pointing to its digest. Storage is recovered later by garbage collection.</p>
        </div>
        <div v-if="deletionPreview.blockedReasons.length" class="deletion-warning">
          <AlertTriangle :size="18" />
          <div>
            <p v-for="reason in deletionPreview.blockedReasons" :key="reason">{{ reason }}</p>
            <code v-for="digest in deletionPreview.relatedArtifacts" :key="digest" class="mt-1 block break-all text-xs">{{ digest }}</code>
          </div>
        </div>
        <div>
          <p class="text-xs text-muted-foreground">Digest</p>
          <code class="mt-1 block break-all text-xs">{{ deletionPreview.digest }}</code>
        </div>
        <div>
          <p class="text-xs text-muted-foreground">Affected tags</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <Badge v-for="tag in deletionPreview.affectedTags" :key="tag">{{ tag }}</Badge>
            <span v-if="!deletionPreview.affectedTags.length" class="text-xs text-muted-foreground">Untagged manifest</span>
          </div>
        </div>
        <label class="field-label">
          Reason <span v-if="deletionPreview.requiresReason">(required by policy)</span>
          <textarea v-model="deletionReason" class="field-control min-h-24 resize-y p-3" maxlength="500" />
        </label>
        <p v-if="deletionError" class="error-text">{{ deletionError }}</p>
        <div class="flex justify-end gap-2">
          <Button variant="ghost" type="button" @click="deletionPreview = null">Cancel</Button>
          <Button
            variant="danger"
            type="submit"
            :disabled="confirmDeletion.isPending.value || deletionPreview.blockedReasons.length > 0 || (deletionPreview.requiresReason && !deletionReason.trim())"
          >
            {{ confirmDeletion.isPending.value ? 'Deleting…' : 'Delete artifact' }}
          </Button>
        </div>
      </form>
    </Dialog>

    <RepositoryPolicyModal
      v-if="policyModal && selectedRepository"
      :project="slug"
      :repository="selectedRepository"
      @close="policyModal = false"
      @saved="policiesSaved"
    />

    <Dialog v-if="lifecyclePreview" labelled-by="lifecycle-review-title" @close="lifecyclePreview = null">
      <section class="modal lifecycle-modal form-stack" aria-labelledby="lifecycle-review-title">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="eyebrow">Lifecycle dry-run</p>
            <h2 id="lifecycle-review-title" class="text-lg font-semibold">{{ lifecyclePreview.repository }}</h2>
            <p class="mt-1 text-xs text-muted-foreground">
              Inventory reconciled {{ new Date(lifecyclePreview.inventoryAt).toLocaleString() }}
              · policy v{{ lifecyclePreview.policyVersion }}
              · evaluator v{{ lifecyclePreview.evaluatorVersion }}
            </p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close lifecycle review" @click="lifecyclePreview = null">
            <X :size="18" />
          </Button>
        </div>

        <div class="lifecycle-summary">
          <div><strong>{{ lifecyclePreview.eligibleCount }}</strong><span>eligible</span></div>
          <div><strong>{{ lifecyclePreview.retainedCount }}</strong><span>retained</span></div>
          <div><strong>{{ lifecyclePreview.blockedCount }}</strong><span>blocked</span></div>
        </div>

        <div class="lifecycle-items">
          <Card v-for="item in lifecyclePreview.items" :key="item.id" class="p-3">
            <div class="flex items-start justify-between gap-3">
              <code class="break-all text-xs">{{ item.digest }}</code>
              <Badge :tone="item.decision === 'eligible' ? 'warning' : item.decision === 'blocked' ? 'danger' : 'neutral'">
                {{ item.decision }}
              </Badge>
            </div>
            <div class="mt-2 flex flex-wrap gap-1">
              <Badge v-for="tag in item.tags" :key="tag">{{ tag }}</Badge>
              <span v-if="!item.tags.length" class="text-xs text-muted-foreground">Untagged</span>
            </div>
            <p class="mt-2 text-xs text-muted-foreground">{{ item.reasons.join(' · ') }}</p>
          </Card>
        </div>

        <div v-if="lifecycleRun" class="deletion-warning">
          <Check :size="18" />
          <p>
            Run {{ lifecycleRun.status }}:
            {{ lifecycleRun.items.filter((item) => item.status === 'deleted').length }} deleted,
            {{ lifecycleRun.items.filter((item) => item.status !== 'deleted').length }} skipped or failed.
            Storage is reclaimed later by garbage collection.
          </p>
        </div>

        <template v-else>
          <label class="field-label">
            Execution reason
            <textarea v-model="lifecycleReason" class="field-control min-h-20 resize-y p-3" maxlength="500" />
          </label>
          <p class="text-xs text-muted-foreground">
            Every candidate is checked again immediately before deletion. Changed artifacts are skipped.
          </p>
          <p v-if="lifecycleError" class="error-text">{{ lifecycleError }}</p>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" @click="lifecyclePreview = null">Cancel</Button>
            <Button
              variant="danger"
              :disabled="!lifecycleReason.trim() || !lifecyclePreview.eligibleCount || runLifecycle.isPending.value"
              @click="runLifecycle.mutate()"
            >
              {{ runLifecycle.isPending.value ? 'Executing…' : `Delete ${lifecyclePreview.eligibleCount} eligible` }}
            </Button>
          </div>
        </template>
      </section>
    </Dialog>

    <Dialog v-if="memberModal && canManage" labelled-by="add-member-title" @close="closeMemberModal">
      <form class="modal form-stack" aria-labelledby="add-member-title" @submit.prevent="submitMember">
        <div class="flex items-start justify-between"><div><h2 id="add-member-title" class="text-lg font-semibold">Add service account</h2><p class="mt-1 text-sm text-muted-foreground">Membership controls token access immediately.</p></div><Button variant="ghost" size="icon" type="button" @click="closeMemberModal"><X :size="18" /></Button></div>
        <label v-if="session.user?.systemAdmin" class="field-label">Principal type<select v-model="memberKind" class="field-control text-sm" @change="changeMemberKind"><option value="service_account">Service account</option><option value="user">User</option></select></label>
        <label class="field-label">Principal<select v-model="memberId" class="field-control text-sm"><option v-for="principal in availablePrincipals" :key="principal.id" :value="principal.id">{{ 'name' in principal ? principal.name : principal.email }} · {{ principal.username }}</option></select></label>
        <label class="field-label">Role<select v-model="memberRole" class="field-control text-sm"><option value="reader">Reader · pull</option><option value="writer">Writer · pull and push</option><option value="admin">Admin · manage project</option></select></label>
        <p v-if="memberError" class="error-text" role="alert">{{ memberError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" @click="closeMemberModal">Cancel</Button><Button type="submit" :disabled="!canManage || !memberId || addMember.isPending.value">Add member</Button></div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.tab-list {
  display: inline-flex;
  gap: .25rem;
  border: 1px solid var(--border);
  border-radius: .75rem;
  background: rgba(255,255,255,.018);
  padding: .25rem;
}
.tab-list button {
  display: flex;
  align-items: center;
  gap: .45rem;
  border: 0;
  border-radius: .55rem;
  background: transparent;
  padding: .55rem .8rem;
  color: var(--muted-foreground);
  font-size: .78rem;
  font-weight: 600;
  cursor: pointer;
}
.tab-list button.active {
  background: var(--muted);
  color: var(--foreground);
  box-shadow: inset 0 0 0 1px rgba(145, 173, 36, 0.08);
}
code {
  color: #c8c1b6;
}

.lifecycle-modal {
  width: min(760px, calc(100vw - 2rem));
  max-height: min(88vh, 900px);
  overflow: auto;
}

.lifecycle-summary {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: .65rem;
}

.lifecycle-summary div {
  display: grid;
  gap: .2rem;
  border: 1px solid var(--border);
  border-radius: .65rem;
  padding: .75rem;
}

.lifecycle-summary strong {
  font-size: 1.15rem;
}

.lifecycle-summary span {
  color: var(--muted-foreground);
  font-size: .72rem;
}

.lifecycle-items {
  display: grid;
  gap: .5rem;
  max-height: 340px;
  overflow: auto;
}

.operation-history {
  display: grid;
  gap: .5rem;
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}

.deletion-warning {
  display: flex;
  gap: .65rem;
  border: 1px solid color-mix(in srgb, var(--danger) 24%, transparent);
  border-radius: .65rem;
  background: color-mix(in srgb, var(--danger) 7%, transparent);
  padding: .8rem;
  color: #eda29d;
  font-size: .78rem;
  line-height: 1.5;
}

.deletion-warning p {
  margin: 0;
}
</style>
