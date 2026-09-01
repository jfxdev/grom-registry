<script setup lang="ts">
import { listServiceAccounts, serviceAccountKeys } from '@/modules/service-accounts/api/serviceAccounts'
import { useSessionStore } from '@/modules/auth/store/session'
import { listUsers, userKeys } from '@/modules/users/api/users'
import {
  createLifecyclePreview,
  executeLifecycle,
  listInventory,
  listLifecycleRuns,
  registryKeys,
} from '@/modules/registry'
import { APIError } from '@/shared/api/client'
import type {
  ArtifactDeletionPreview,
  AccountedStorageUsage,
  LifecyclePreview,
  LifecycleRun,
  ManifestInventory,
  ManifestPlatform,
  Membership,
  PrincipalKind,
  ProjectRole,
  Repository,
  RepositoryPolicySet,
} from '@/shared/api/models'
import { Accordion } from '@/shared/components/ui/accordion'
import { Badge } from '@/shared/components/ui/badge'
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@/shared/components/ui/breadcrumb'
import { ActionButton, Button, DeleteButton } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { DangerZone } from '@/shared/components/ui/danger-zone'
import { Dialog } from '@/shared/components/ui/dialog'
import { Input } from '@/shared/components/ui/input'
import { PrincipalTypeBadge } from '@/shared/components/ui/principal-type-badge'
import { DockerPushBanner } from '@/shared/components/registry'
import { PaginationControls } from '@/shared/components/ui/pagination'
import { ROUTES } from '@/shared/constants'
import { writeClipboardText } from '@/shared/lib/clipboard'
import { pageItems, useCursorPagination } from '@/shared/lib/pagination'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { AlertTriangle, Archive, ArchiveRestore, Box, Check, Clipboard, HardDrive, Pencil, Plus, RefreshCw, Search, Settings2, ShieldCheck, Tags, Terminal, Trash2, Users, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
	archiveRepository,
  deleteArtifact,
  deleteMember,
	deleteProject,
	getRepository,
	getProject,
  listArtifactDeletions,
  listMembers,
  listRepositories,
  listTags,
  projectKeys,
  previewArtifactDeletion,
	removeRepository,
	setMember,
	unarchiveRepository,
} from '../api/projects'
import RepositoryCreateModal from '../components/RepositoryCreateModal.vue'
import RepositoryPolicyModal from '../components/RepositoryPolicyModal.vue'

const route = useRoute()
const router = useRouter()
const queryClient = useQueryClient()
const session = useSessionStore()
const slug = computed(() => String(route.params.project))
const repositoryId = computed(() => String(route.params.repositoryId ?? ''))
const registryHost = window.location.host
const selectedRepository = ref<Repository | null>(null)
const repositoryModal = ref(false)
const policyModal = ref(false)
const pushInstructionsOpen = ref(false)
const memberModal = ref(false)
const memberId = ref('')
const memberKind = ref<PrincipalKind>('service_account')
const memberRole = ref<ProjectRole>('reader')
const memberError = ref('')
const memberToEdit = ref<Membership | null>(null)
const editedMemberRole = ref<ProjectRole>('reader')
const membershipToRemove = ref<Membership | null>(null)
const copied = ref('')
const copyError = ref('')
const deletionPreview = ref<ArtifactDeletionPreview | null>(null)
const deletionTrigger = ref<globalThis.HTMLElement | null>(null)
const deletionReference = ref('')
const deletionReason = ref('')
const deletionError = ref('')
const lifecyclePreview = ref<LifecyclePreview | null>(null)
const lifecycleRun = ref<LifecycleRun | null>(null)
const lifecycleReason = ref('')
const lifecycleError = ref('')
const projectDeletionOpen = ref(false)
const projectSettingsTrigger = ref<globalThis.HTMLElement | null>(null)
const projectSettingsOpen = ref(false)
const projectDeletionError = ref('')
const selectedManifest = ref<ManifestInventory | null>(null)
const repositoryOperationError = ref('')
const archiveRepositoryOpen = ref(false)
const removeRepositoryOpen = ref(false)
const repositoryPagination = useCursorPagination()
const memberPagination = useCursorPagination()
const memberSearch = ref('')
const tagPagination = useCursorPagination()
const inventoryPagination = useCursorPagination()
const deletionPagination = useCursorPagination()
const lifecyclePagination = useCursorPagination()
const activeRepositoryOperations = ref(0)
const repositoryOperationActive = computed(() => activeRepositoryOperations.value > 0)

function openProjectSettings(event: globalThis.MouseEvent) {
  projectSettingsTrigger.value = event.currentTarget instanceof globalThis.HTMLElement ? event.currentTarget : null
  projectSettingsOpen.value = true
}

function openProjectDeletion() {
  projectSettingsOpen.value = false
  projectDeletionOpen.value = true
}

const project = useQuery({ queryKey: computed(() => projectKeys.detail(slug.value)), queryFn: () => getProject(slug.value) })
const repositories = useQuery({ queryKey: computed(() => [...projectKeys.repositories(slug.value), repositoryPagination.cursor.value]), queryFn: () => listRepositories(slug.value, repositoryPagination.cursor.value) })
const repositoriesError = computed(() =>
  repositories.error.value instanceof APIError ? repositories.error.value.message : 'Could not load repositories. Please try again.',
)
const members = useQuery({ queryKey: computed(() => [...projectKeys.members(slug.value), memberSearch.value.trim(), memberPagination.cursor.value]), queryFn: () => listMembers(slug.value, memberPagination.cursor.value, memberSearch.value.trim()), enabled: computed(() => session.user?.systemViewer !== true) })
const accounts = useQuery({ queryKey: serviceAccountKeys.list(), queryFn: () => listServiceAccounts(), enabled: computed(() => session.user?.systemViewer !== true) })
const canManage = computed(() =>
  session.user?.systemViewer !== true && project.data.value?.canManage === true,
)

async function handleRepositoryCreated() {
  repositoryModal.value = false
  repositoryPagination.reset()
  await repositories.refetch()
}

const users = useQuery({ queryKey: userKeys.all, queryFn: () => listUsers(), enabled: computed(() => canManage.value) })
const routedRepository = useQuery({
  queryKey: computed(() => [...projectKeys.repositories(slug.value), repositoryId.value]),
  queryFn: () => getRepository(slug.value, repositoryId.value),
  enabled: computed(() => Boolean(repositoryId.value) && repositories.isSuccess.value && !pageItems(repositories.data.value).some((repository) => repository.id === repositoryId.value)),
})
const tags = useQuery({
  queryKey: computed(() => [...projectKeys.tags(slug.value, selectedRepository.value?.name ?? ''), tagPagination.cursor.value]),
  queryFn: () => listTags(slug.value, selectedRepository.value!.name, tagPagination.cursor.value),
  enabled: computed(() => selectedRepository.value !== null),
  refetchInterval: computed(() => repositoryOperationActive.value ? 5_000 : false),
  refetchOnWindowFocus: computed(() => repositoryOperationActive.value),
})
const artifactDeletionHistory = useQuery({
  queryKey: computed(() => [...projectKeys.artifactDeletions(slug.value, selectedRepository.value?.name ?? ''), deletionPagination.cursor.value]),
  queryFn: () => listArtifactDeletions(slug.value, selectedRepository.value!.name, deletionPagination.cursor.value),
  enabled: computed(() => canManage.value && selectedRepository.value !== null),
})
const lifecycleHistory = useQuery({
  queryKey: computed(() => [...registryKeys.lifecycleRuns(slug.value, selectedRepository.value?.name ?? ''), lifecyclePagination.cursor.value]),
  queryFn: () => listLifecycleRuns(slug.value, selectedRepository.value!.name, lifecyclePagination.cursor.value),
  enabled: computed(() => canManage.value && selectedRepository.value !== null),
})
const inventory = useQuery({
  queryKey: computed(() => [...registryKeys.inventory(slug.value, selectedRepository.value?.name ?? ''), inventoryPagination.cursor.value]),
  queryFn: () => listInventory(slug.value, selectedRepository.value!.name, inventoryPagination.cursor.value),
  enabled: computed(() => selectedRepository.value !== null),
  refetchInterval: computed(() => repositoryOperationActive.value ? 5_000 : false),
  refetchOnWindowFocus: computed(() => repositoryOperationActive.value),
})

const currentInventoryItems = computed(() =>
  pageItems(inventory.data.value).filter((manifest) => manifest.state === 'active' || manifest.state === 'untagged'),
)
const historicalInventoryItems = computed(() =>
  pageItems(inventory.data.value).filter((manifest) => manifest.state === 'missing' || manifest.state === 'deleted'),
)
const memberGroups = computed(() => {
  const items = pageItems(members.data.value)
  return [
    { kind: 'user' as const, label: 'Users', members: items.filter((member) => member.principalKind === 'user') },
    { kind: 'service_account' as const, label: 'Service accounts', members: items.filter((member) => member.principalKind === 'service_account') },
  ].filter((group) => group.members.length)
})

watch([repositoryId, () => repositories.data.value, () => routedRepository.data.value], ([id, page, routed]) => {
  if (!id) {
    selectedRepository.value = null
    return
  }
  selectedRepository.value = pageItems(page).find((repository) => repository.id === id) ?? routed ?? null
}, { immediate: true })

watch(repositoryId, () => {
  tagPagination.reset()
  inventoryPagination.reset()
  deletionPagination.reset()
  lifecyclePagination.reset()
})

watch(memberSearch, () => memberPagination.reset())

async function openRepository(repository: Repository) {
  await router.push({ name: 'repository-detail', params: { project: slug.value, repositoryId: repository.id } })
}

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
const changeMemberRole = useMutation({
  mutationFn: () => setMember(slug.value, memberToEdit.value!.principalKind, memberToEdit.value!.principalId, editedMemberRole.value),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.members(slug.value) })
    memberError.value = ''
    memberToEdit.value = null
  },
  onError: (caught) => {
    memberError.value = caught instanceof APIError ? caught.message : 'Could not change this member role'
  },
})
const removeMember = useMutation({
  mutationFn: () => deleteMember(slug.value, membershipToRemove.value!.principalKind, membershipToRemove.value!.principalId),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.members(slug.value) })
    membershipToRemove.value = null
    memberError.value = ''
  },
  onError: (caught) => {
    memberError.value = caught instanceof APIError ? caught.message : 'Could not remove this project member'
  },
})
const archive = useMutation({
  mutationFn: () => archiveRepository(slug.value, selectedRepository.value!.id),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    selectedRepository.value = selectedRepository.value ? { ...selectedRepository.value, status: 'archived' } : null
    repositoryOperationError.value = ''
		archiveRepositoryOpen.value = false
  },
  onError: (caught) => { repositoryOperationError.value = caught instanceof APIError ? caught.message : 'Could not archive this repository' },
})
const unarchive = useMutation({
  mutationFn: () => unarchiveRepository(slug.value, selectedRepository.value!.id),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    selectedRepository.value = selectedRepository.value ? { ...selectedRepository.value, status: 'empty' } : null
    repositoryOperationError.value = ''
  },
  onError: (caught) => { repositoryOperationError.value = caught instanceof APIError ? caught.message : 'Could not unarchive this repository' },
})
const removeLogicalRepository = useMutation({
  mutationFn: () => removeRepository(slug.value, selectedRepository.value!.id),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    selectedRepository.value = null
    removeRepositoryOpen.value = false
    repositoryOperationError.value = ''
  },
  onError: (caught) => { repositoryOperationError.value = caught instanceof APIError ? caught.message : 'Could not remove this repository' },
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

function requestArtifactDeletionPreview(event: globalThis.MouseEvent, tag: string) {
  deletionTrigger.value = event.currentTarget instanceof globalThis.HTMLElement ? event.currentTarget : null
  previewDeletion.mutate(tag)
}

const confirmDeletion = useMutation({
  onMutate: () => { activeRepositoryOperations.value++ },
  mutationFn: () => deleteArtifact(slug.value, {
    repository: deletionPreview.value!.repository,
    reference: deletionReference.value,
    reason: deletionReason.value,
    expectedDigest: deletionPreview.value!.digest,
    expectedTags: deletionPreview.value!.affectedTags,
    expectedChildDigests: deletionPreview.value!.childDigests,
  }),
  onSuccess: async (deletion) => {
    const repository = deletion.repository
	await queryClient.invalidateQueries({ queryKey: projectKeys.detail(slug.value) })
	await queryClient.invalidateQueries({ queryKey: projectKeys.tags(slug.value, repository) })
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    await queryClient.invalidateQueries({ queryKey: registryKeys.inventory(slug.value, repository) })
    await queryClient.invalidateQueries({
      queryKey: projectKeys.artifactDeletions(slug.value, repository),
    })
    deletionPreview.value = null
    deletionError.value = deletion.status === 'failed'
      ? deletion.message || 'The deletion only partially completed. Repository state was refreshed.'
      : ''
  },
  onError: (caught) => {
    deletionError.value = caught instanceof APIError ? caught.message : 'Could not delete this artifact'
  },
  onSettled: () => { activeRepositoryOperations.value-- },
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
  onMutate: () => { activeRepositoryOperations.value++ },
  mutationFn: () => executeLifecycle(slug.value, lifecyclePreview.value!.id, lifecycleReason.value),
  onSuccess: async (run) => {
    const repository = run.repository
    lifecycleRun.value = run
    lifecycleError.value = ''
	await queryClient.invalidateQueries({ queryKey: projectKeys.detail(slug.value) })
	await queryClient.invalidateQueries({ queryKey: projectKeys.tags(slug.value, repository) })
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(slug.value) })
    await queryClient.invalidateQueries({ queryKey: registryKeys.inventory(slug.value, repository) })
    await queryClient.invalidateQueries({ queryKey: registryKeys.lifecycleRuns(slug.value, repository) })
  },
  onError: (caught) => {
    lifecycleError.value = caught instanceof APIError ? caught.message : 'Could not execute lifecycle'
  },
  onSettled: () => { activeRepositoryOperations.value-- },
})

const availableAccounts = computed(() => {
  return pageItems(accounts.data.value)
})
const availableUsers = computed(() => {
  return pageItems(users.data.value)
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

async function copyCommand(command: string, key: string) {
  const result = await writeClipboardText(command)
  if (result !== 'copied') {
    copyError.value = 'Could not copy the command. Select and copy it manually.'
    return
  }
  copyError.value = ''
  copied.value = key
  window.setTimeout(() => { copied.value = '' }, 1200)
}

function pullCommand(repository: string, tag = 'latest') {
  return `docker pull ${window.location.host}/${slug.value}/${repository}:${tag}`
}

function manifestTags(manifest: ManifestInventory) {
  return manifest.tags ?? []
}

function manifestStateLabel(state: ManifestInventory['state']) {
  if (state === 'active') return 'Active'
  if (state === 'untagged') return 'Live, untagged'
  if (state === 'missing') return 'Missing from Distribution'
  return 'Deleted'
}

function manifestStateTone(state: ManifestInventory['state']): 'neutral' | 'success' | 'warning' | 'danger' {
  if (state === 'active') return 'success'
  if (state === 'untagged') return 'warning'
  if (state === 'missing') return 'danger'
  return 'neutral'
}

function manifestPresence(manifest: ManifestInventory) {
  return manifestTags(manifest).length ? manifestTags(manifest).join(', ') : manifestStateLabel(manifest.state)
}

const manifestsByTag = computed(() => {
  const lookup = new Map<string, ManifestInventory>()
  for (const manifest of pageItems(inventory.data.value)) {
    for (const tag of manifestTags(manifest)) lookup.set(tag, manifest)
  }
  return lookup
})

function manifestForTag(tag: string) {
  return manifestsByTag.value.get(tag)
}

function platformsForTag(tag: string): ManifestPlatform[] {
  return manifestForTag(tag)?.platforms ?? []
}

function lastPushedLabel(tag: string): string | null {
  const manifest = manifestForTag(tag)
  if (!manifest?.lastPushedAt) return null
  const days = Math.floor((Date.now() - new Date(manifest.lastPushedAt).getTime()) / 86_400_000)
  const when = days <= 0 ? 'today' : days === 1 ? '1 day ago' : `${days} days ago`
  return manifest.lastPushedBy ? `Last pushed ${when} by ${manifest.lastPushedBy}` : `Last pushed ${when}`
}

function formatCompressedSize(bytes: number | undefined) {
  if (bytes === undefined || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / (1024 ** index)
  return `${value.toLocaleString(undefined, { maximumFractionDigits: index === 0 ? 0 : 2 })} ${units[index]}`
}

function formatAccountedBytes(bytes: number | null | undefined) {
  if (bytes === null || bytes === undefined) return '—'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / (1024 ** index)).toLocaleString(undefined, { maximumFractionDigits: index === 0 ? 0 : 2 })} ${units[index]}`
}

function accountedUsageLabel(usage: AccountedStorageUsage | undefined) {
  if (!usage) return 'Accounting pending'
  if (usage.status === 'pending') return 'Accounting pending'
  if (usage.status === 'unavailable') return 'Accounting unavailable'
  return formatAccountedBytes(usage.accountedBytes)
}

function editMember(member: Membership) {
  memberToEdit.value = member
  editedMemberRole.value = member.role
  memberError.value = ''
}

function openMemberModal() {
  if (!canManage.value) return
  memberKind.value = 'service_account'
  memberId.value = availableAccounts.value[0]?.id ?? ''
	memberRole.value = 'reader'
  memberError.value = ''
  memberModal.value = true
}

function closeMemberModal() {
  memberModal.value = false
  memberError.value = ''
}

function closeChangeMemberRoleModal() {
  if (changeMemberRole.isPending.value) return
  memberToEdit.value = null
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

const policyTypeLabels: Record<string, string> = {
  retention: 'Retention',
  tag_protection: 'Tag protection',
  immutability: 'Immutability',
  tag_naming: 'Tag naming',
  manual_deletion: 'Manual deletion',
}

function policyTypeLabel(type: string) {
  return policyTypeLabels[type] ?? type
}

function retentionCriterionActive(enabled: boolean | undefined, limit: number | undefined) {
  return (enabled ?? limit !== undefined) && limit !== undefined
}

function tagScopeSuffix(patterns: string[] | undefined) {
  return patterns?.length ? ` matching ${patterns.join(', ')}` : ''
}

function policySummary(policy: Repository['policies'][number]) {
  const scope = tagScopeSuffix(policy.tagPatterns)
  if (policy.type === 'retention') {
    const sentences: string[] = []
    if (retentionCriterionActive(policy.expireAfterDaysEnabled, policy.expireAfterDays)) sentences.push(`Removes tags${scope} older than ${policy.expireAfterDays} days`)
    if (retentionCriterionActive(policy.keepLastEnabled, policy.keepLast)) sentences.push(`Always keeps the last ${policy.keepLast} tags${scope}`)
    if (retentionCriterionActive(policy.untaggedGraceDaysEnabled, policy.untaggedGraceDays)) sentences.push(`Cleans untagged images after ${policy.untaggedGraceDays} days`)
    return sentences.length ? `${sentences.join('. ')}.` : 'No active criteria.'
  }
  if (policy.type === 'tag_protection') {
    const actions: string[] = []
    if (policy.preventDeletion) actions.push('deletion')
    if (policy.preventOverwrite) actions.push('overwrite')
    if (policy.excludeFromLifecycle) actions.push('automatic lifecycle cleanup')
    return actions.length ? `Protects tags${scope} from ${actions.join(', ')}.` : `No active rules for tags${scope}.`
  }
  if (policy.type === 'immutability') {
    return policy.preventOverwrite ? `Blocks tag overwrite for tags${scope}.` : `No active rules for tags${scope}.`
  }
  if (policy.type === 'tag_naming') {
    return policy.allowedPatterns?.length ? `Only allows tags matching ${policy.allowedPatterns.join(', ')}.` : 'No naming restriction.'
  }
  return policy.requireReason ? 'Requires a reason before manual deletion.' : 'No reason required for manual deletion.'
}
</script>

<template>
  <div class="page-shell">
    <Breadcrumb v-if="!repositoryId" class="mb-5">
      <BreadcrumbList>
        <BreadcrumbItem>
          <BreadcrumbLink as="router-link" :to="ROUTES.projects">Projects</BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        <BreadcrumbItem>
          <BreadcrumbPage>{{ project.data.value?.name ?? slug }}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
    <header v-if="!repositoryId" class="page-header">
      <div>
        <p class="eyebrow">Project namespace</p>
        <h1 class="page-title">{{ project.data.value?.name ?? slug }}</h1>
        <p class="project-registry-url font-mono">{{ registryHost }}/{{ slug }}/</p>
        <p class="mt-2 text-sm text-muted-foreground">
          Accounted registry usage: <strong class="text-foreground">{{ project.data.value ? accountedUsageLabel(project.data.value.accountedUsage) : 'Loading…' }}</strong>
          <span v-if="project.data.value?.accountedUsage?.status === 'stale'"> — last successful accounting is stale.</span>
        </p>
        <p class="mt-1 text-xs text-muted-foreground">Shared descriptors count once in this project. Physical installation storage is shown in Settings.</p>
      </div>
      <div class="flex items-center gap-2">
        <Badge tone="success">Active</Badge>
        <Button variant="outline" size="sm" @click="pushInstructionsOpen = true"><Terminal :size="14" /> Instructions</Button>
        <Button v-if="canManage" size="sm" @click="repositoryModal = true"><Plus :size="15" /> New repository</Button>
        <Button
          v-if="canManage"
          variant="outline"
          size="icon"
          aria-label="Project settings"
          @click="openProjectSettings($event)"
        >
          <Settings2 :size="16" />
        </Button>
      </div>
    </header>

    <section v-if="!repositoryId">
      <div v-if="repositories.isError.value" class="empty-state mt-5" role="alert">
        <div>
          <Box class="mx-auto mb-3 text-destructive" :size="28" />
          <p class="font-medium text-foreground">Could not load repositories</p>
          <p class="mt-2 text-sm">{{ repositoriesError }}</p>
          <Button class="mt-4" variant="outline" size="sm" @click="repositories.refetch()">Try again</Button>
        </div>
      </div>
      <div v-else-if="!pageItems(repositories.data.value).length" class="empty-state mt-5">
        <div>
          <Box class="mx-auto mb-3 text-accent" :size="28" />
          <p class="font-medium text-foreground">No repositories yet</p>
          <p class="mt-2 text-sm">A Writer or Admin can create one automatically with the first push.</p>
        </div>
      </div>
      <div v-else class="table-shell mt-5">
        <button v-for="repository in pageItems(repositories.data.value)" :key="repository.id" class="data-row w-full text-left" @click="openRepository(repository)">
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
            <p class="text-xs text-muted-foreground">{{ accountedUsageLabel(repository.accountedUsage) }} accounted<span v-if="repository.accountedUsage?.status === 'stale'"> · stale</span></p>
          </div>
          <span class="flex items-center gap-2 text-accent"><Settings2 :size="14" /> →</span>
        </button>
      </div>
      <PaginationControls
        :page="repositoryPagination.page.value"
        :has-previous="repositoryPagination.hasPrevious.value"
        :has-next="Boolean(repositories.data.value?.nextCursor)"
        :disabled="repositories.isFetching.value"
        @previous="repositoryPagination.previous()"
        @next="repositoryPagination.next(repositories.data.value?.nextCursor)"
      />
    </section>

    <Dialog
      v-if="projectSettingsOpen && canManage"
      labelled-by="project-settings-title"
      @close="projectSettingsOpen = false"
    >
      <section class="modal form-stack project-settings-modal" aria-labelledby="project-settings-title">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="eyebrow">Project</p>
            <h2 id="project-settings-title" class="text-lg font-semibold">Project settings</h2>
            <p class="mt-1 text-sm text-muted-foreground">Manage sensitive project-level actions.</p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close project settings" @click="projectSettingsOpen = false">
            <X :size="18" />
          </Button>
        </div>
        <section class="settings-members">
          <div class="members-toolbar">
            <div class="flex items-center gap-2">
              <Users :size="16" class="text-muted-foreground" />
              <div>
                <h3 class="text-sm font-semibold">Members</h3>
                <p class="mt-1 text-xs text-muted-foreground">Control who can access this project.</p>
              </div>
            </div>
            <div class="members-toolbar-actions">
              <div class="member-search">
                <Search :size="15" aria-hidden="true" />
                <Input v-model="memberSearch" type="search" placeholder="Search members" aria-label="Search members" />
              </div>
              <Button size="sm" @click="openMemberModal"><Plus :size="15" /> Add member</Button>
            </div>
          </div>
          <div v-if="!pageItems(members.data.value).length" class="mt-3 rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
            {{ memberSearch.trim() ? 'No members match this search.' : 'No members are assigned to this project.' }}
          </div>
          <div v-else class="member-tables mt-3">
            <section v-for="group in memberGroups" :key="group.kind" class="member-table-group" :aria-labelledby="`member-group-${group.kind}`">
              <div class="member-table-group-heading">
                <h4 :id="`member-group-${group.kind}`">{{ group.label }}</h4>
                <span>{{ group.members.length }} on this page</span>
              </div>
              <div class="member-table">
                <div class="member-table-head" aria-hidden="true">
                  <span>Member</span><span>Type</span><span>Role</span><span>Assigned</span><span>Actions</span>
                </div>
                <div v-for="member in group.members" :key="`${member.principalKind}:${member.principalId}`" class="member-row">
                  <div class="member-identity"><p class="text-sm font-semibold">{{ member.principalName }}</p><p class="mt-1 text-xs text-muted-foreground">{{ member.principalDetail }}</p></div>
                  <PrincipalTypeBadge class="member-type" :kind="member.principalKind" :icon-only="true" />
                  <Badge class="member-role">{{ member.role }}</Badge>
                  <p class="member-assigned text-xs text-muted-foreground">{{ new Date(member.createdAt).toLocaleDateString() }}</p>
                  <div class="member-actions flex gap-1">
                    <ActionButton variant="cyan" size="icon" :aria-label="`Change role for ${member.principalName}`" @click="editMember(member)"><Pencil :size="15" /></ActionButton>
                    <DeleteButton size="icon" :aria-label="`Remove ${member.principalKind === 'service_account' ? 'service account' : 'user'} member`" @click="memberError = ''; membershipToRemove = member" />
                  </div>
                </div>
              </div>
            </section>
          </div>
          <PaginationControls
            :page="memberPagination.page.value"
            :has-previous="memberPagination.hasPrevious.value"
            :has-next="Boolean(members.data.value?.nextCursor)"
            :disabled="members.isFetching.value"
            @previous="memberPagination.previous()"
            @next="memberPagination.next(members.data.value?.nextCursor)"
          />
        </section>
        <DangerZone
          v-if="session.user?.systemAdmin"
          title="Danger zone"
          description="Delete this project only after all logical repositories have been removed."
        >
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm font-semibold">Delete project</p>
              <p class="mt-1 text-xs text-muted-foreground">This permanently removes the project and its memberships.</p>
            </div>
            <DeleteButton size="sm" @click="openProjectDeletion">Delete project</DeleteButton>
          </div>
        </DangerZone>
      </section>
    </Dialog>

    <Dialog
      v-if="projectDeletionOpen && session.user?.systemAdmin"
      labelled-by="delete-project-title"
      :restore-focus="projectSettingsTrigger"
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
          <DeleteButton type="submit" :disabled="removeProject.isPending.value">
            {{ removeProject.isPending.value ? 'Deleting…' : 'Delete project' }}
          </DeleteButton>
        </div>
      </form>
    </Dialog>

    <section v-if="repositoryId && selectedRepository" class="repository-detail form-stack" aria-labelledby="repository-details-title">
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink as="router-link" :to="ROUTES.projects">Projects</BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbLink as="router-link" :to="{ name: 'project-detail', params: { project: slug } }">{{ project.data.value?.name ?? slug }}</BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{{ selectedRepository.name }}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <header class="page-header">
        <div>
          <p class="eyebrow">Repository</p>
          <h1 id="repository-details-title" class="page-title">{{ selectedRepository.name }}</h1>
          <p class="page-description">
            {{ selectedRepository.profileSource === 'inferred'
              ? `${selectedRepository.profileConfidence} confidence · inferred from uploaded content`
              : 'Waiting for the first identifiable upload' }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="sm" @click="pushInstructionsOpen = true"><Terminal :size="14" /> Instructions</Button>
          <Button
            v-if="canManage"
            variant="outline"
            size="sm"
            :disabled="reviewLifecycle.isPending.value"
            @click="reviewLifecycle.mutate()"
          >
            <RefreshCw :size="14" /> {{ reviewLifecycle.isPending.value ? 'Reviewing…' : 'Review lifecycle' }}
          </Button>
          <Button v-if="canManage && selectedRepository.status !== 'archived'" variant="outline" size="sm" :disabled="archive.isPending.value" @click="repositoryOperationError = ''; archiveRepositoryOpen = true">
            <Archive :size="14" /> Archive
          </Button>
          <Button v-if="canManage && selectedRepository.status === 'archived'" variant="outline" size="sm" :disabled="unarchive.isPending.value" @click="unarchive.mutate()">
            <ArchiveRestore :size="14" /> {{ unarchive.isPending.value ? 'Unarchiving…' : 'Unarchive' }}
          </Button>
          <DeleteButton v-if="canManage && selectedRepository.status === 'archived'" size="sm" @click="repositoryOperationError = ''; removeRepositoryOpen = true">Remove logical record</DeleteButton>
        </div>
      </header>

      <div v-if="selectedRepository.profileNeedsReview" class="operation-warning" role="alert">
        <AlertTriangle :size="18" />
        <div>
          <strong>Profile needs review</strong>
          <p>Different primary artifact types were detected in this repository.</p>
        </div>
      </div>

      <section class="overview-grid" aria-label="Repository overview">
        <Card class="overview-card">
          <div class="overview-icon"><Box :size="18" /></div>
          <div>
            <p class="overview-label">Profile</p>
            <p class="overview-value">{{ profileLabel(selectedRepository.profile) }}</p>
            <p class="overview-detail">{{ selectedRepository.profileSource === 'inferred' ? `${selectedRepository.profileConfidence} confidence` : 'Not yet inferred' }}</p>
          </div>
        </Card>
        <Card class="overview-card">
          <div class="overview-icon"><ShieldCheck :size="18" /></div>
          <div>
            <p class="overview-label">Status</p>
            <p class="overview-value capitalize">{{ selectedRepository.status }}</p>
            <p class="overview-detail">{{ selectedRepository.creationSource === 'push' ? 'Created by push' : 'Created manually' }}</p>
          </div>
        </Card>
        <Card class="overview-card">
          <div class="overview-icon"><HardDrive :size="18" /></div>
          <div>
            <p class="overview-label">Accounted usage</p>
            <p class="overview-value">{{ accountedUsageLabel(selectedRepository.accountedUsage) }}</p>
            <p class="overview-detail">
              <template v-if="selectedRepository.accountedUsage?.reconciledAt">Last reconciled {{ new Date(selectedRepository.accountedUsage.reconciledAt!).toLocaleString() }}</template>
              <template v-else>Logical descriptor usage, not reclaimable physical storage.</template>
              <template v-if="selectedRepository.accountedUsage?.status === 'stale'"> · stale</template>
            </p>
          </div>
        </Card>
        <Card class="overview-card">
          <div class="overview-icon"><Tags :size="18" /></div>
          <div>
            <p class="overview-label">Policies</p>
            <p class="overview-value">{{ selectedRepository.policies.filter((policy) => policy.enabled).length }}/{{ selectedRepository.policies.length }} enabled</p>
            <p class="overview-detail">Behavior policies applied to this repository.</p>
          </div>
        </Card>
      </section>

      <section class="repo-panel">
        <div class="panel-heading">
          <div>
            <h2>Tags</h2>
            <p>Pull commands and platform manifests for each published tag.</p>
          </div>
        </div>
        <div class="repo-panel-body">
          <div v-if="!pageItems(tags.data.value).length" class="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">No tags available.</div>
          <div v-else class="space-y-3">
            <Card v-for="tag in pageItems(tags.data.value)" :key="tag" class="overflow-hidden">
              <div class="flex items-start justify-between gap-3 border-b px-4 py-3">
                <div>
                  <p class="eyebrow">Tag</p>
                  <code class="mt-1 block text-base font-semibold text-accent">{{ tag }}</code>
                  <p v-if="lastPushedLabel(tag)" class="mt-1 text-xs text-muted-foreground">{{ lastPushedLabel(tag) }}</p>
                </div>
                <div class="flex shrink-0 gap-1">
                  <Button variant="ghost" size="sm" @click="copyCommand(pullCommand(selectedRepository!.name, tag), `pull:${tag}`)">
                    <Check v-if="copied === `pull:${tag}`" :size="14" /><Clipboard v-else :size="14" /> Copy pull
                  </Button>
                  <Button
                    v-if="canManage"
                    variant="ghost"
                    size="icon"
                    :aria-label="`Delete ${tag}`"
                    :disabled="previewDeletion.isPending.value"
                    @click="requestArtifactDeletionPreview($event, tag)"
                  >
                    <Trash2 :size="15" />
                  </Button>
                </div>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full min-w-[580px] text-sm">
                  <thead class="border-b text-left text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    <tr>
                      <th class="px-4 py-3">Digest</th>
                      <th class="px-4 py-3">OS/ARCH</th>
                      <th class="px-4 py-3 text-right">Compressed size</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="platform in platformsForTag(tag)" :key="platform.digest" class="border-b last:border-0">
                      <td class="px-4 py-3"><code class="text-xs text-accent">{{ platform.digest || manifestForTag(tag)?.digest || '—' }}</code></td>
                      <td class="px-4 py-3 text-muted-foreground">{{ platform.os }}/{{ platform.architecture }}{{ platform.variant ? `/${platform.variant}` : '' }}</td>
                      <td class="px-4 py-3 text-right tabular-nums">{{ formatCompressedSize(platform.compressedSize) }}</td>
                    </tr>
                    <tr v-if="!platformsForTag(tag).length">
                      <td class="px-4 py-3"><code class="text-xs text-accent">{{ manifestForTag(tag)?.digest ?? '—' }}</code></td>
                      <td class="px-4 py-3 text-muted-foreground">—</td>
                      <td class="px-4 py-3 text-right tabular-nums">—</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </Card>
          </div>
          <PaginationControls
            :page="tagPagination.page.value"
            :has-previous="tagPagination.hasPrevious.value"
            :has-next="Boolean(tags.data.value?.nextCursor)"
            :disabled="tags.isFetching.value"
            @previous="tagPagination.previous()"
            @next="tagPagination.next(tags.data.value?.nextCursor)"
          />
        </div>
      </section>

      <Accordion title="Manifest inventory" :description="`${pageItems(inventory.data.value).length} observed manifests`" default-open>
        <p v-if="inventory.isLoading.value" class="text-xs text-muted-foreground">Loading manifest inventory…</p>
        <p v-else-if="!pageItems(inventory.data.value).length" class="text-xs text-muted-foreground">No observed manifests yet.</p>
        <template v-if="currentInventoryItems.length">
          <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Current in Distribution</p>
          <Card v-for="manifest in currentInventoryItems" :key="manifest.id" class="cursor-pointer p-3" role="button" tabindex="0" @click="selectedManifest = manifest" @keydown.enter="selectedManifest = manifest">
            <div class="flex items-center justify-between gap-3"><code class="truncate text-xs">{{ manifest.digest }}</code><div class="flex gap-1"><Badge>{{ manifest.observedKind.replaceAll('_', ' ') }}</Badge><Badge :tone="manifestStateTone(manifest.state)">{{ manifestStateLabel(manifest.state) }}</Badge></div></div>
            <p class="mt-1 text-xs text-muted-foreground">{{ manifestPresence(manifest) }} · {{ formatCompressedSize(manifest.manifestSize) }} manifest metadata</p>
          </Card>
        </template>
        <template v-if="historicalInventoryItems.length">
          <p class="mt-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">History</p>
          <Card v-for="manifest in historicalInventoryItems" :key="manifest.id" class="cursor-pointer border-dashed p-3" role="button" tabindex="0" @click="selectedManifest = manifest" @keydown.enter="selectedManifest = manifest">
            <div class="flex items-center justify-between gap-3"><code class="truncate text-xs">{{ manifest.digest }}</code><Badge :tone="manifestStateTone(manifest.state)">{{ manifestStateLabel(manifest.state) }}</Badge></div>
            <p class="mt-1 text-xs text-muted-foreground">Historical record · {{ formatCompressedSize(manifest.manifestSize) }} manifest metadata</p>
          </Card>
        </template>
        <PaginationControls
          :page="inventoryPagination.page.value"
          :has-previous="inventoryPagination.hasPrevious.value"
          :has-next="Boolean(inventory.data.value?.nextCursor)"
          :disabled="inventory.isFetching.value"
          @previous="inventoryPagination.previous()"
          @next="inventoryPagination.next(inventory.data.value?.nextCursor)"
        />
      </Accordion>

      <section class="repo-panel">
        <div class="panel-heading">
          <div>
            <h2>Policies</h2>
            <p>Retention, protection, and naming rules applied to this repository.</p>
          </div>
          <Button v-if="canManage" variant="outline" size="sm" @click="policyModal = true">Manage</Button>
        </div>
        <div class="repo-panel-body">
          <p v-if="!selectedRepository.policies.length" class="text-xs text-muted-foreground">No behavior policies configured.</p>
          <ul v-else class="policy-summary-list">
            <li
              v-for="policy in selectedRepository.policies"
              :key="policy.id"
              class="policy-summary-item"
              :class="{ 'policy-summary-item-enabled': policy.enabled }"
            >
              <div class="flex items-center justify-between gap-2">
                <strong class="text-sm">{{ policyTypeLabel(policy.type) }}</strong>
                <Badge :tone="policy.enabled ? 'success' : 'neutral'">{{ policy.enabled ? 'Enabled' : 'Disabled' }}</Badge>
              </div>
              <p class="mt-1 text-xs text-muted-foreground">{{ policySummary(policy) }}</p>
            </li>
          </ul>
        </div>
      </section>

      <p v-if="copyError" class="error-text" role="alert">{{ copyError }}</p>
      <p v-if="repositoryOperationError" class="error-text" role="alert">{{ repositoryOperationError }}</p>
      <p v-if="deletionError && !deletionPreview" class="error-text">{{ deletionError }}</p>
      <p v-if="lifecycleError && !lifecyclePreview" class="error-text">{{ lifecycleError }}</p>

      <section v-if="canManage && (pageItems(artifactDeletionHistory.data.value).length || pageItems(lifecycleHistory.data.value).length)" class="repo-panel">
        <div class="panel-heading">
          <div>
            <h2>Deletion history</h2>
            <p>Manual deletions and lifecycle runs executed against this repository.</p>
          </div>
        </div>
        <div class="repo-panel-body">
          <Card v-for="deletion in pageItems(artifactDeletionHistory.data.value)" :key="deletion.id" class="p-3">
            <div class="flex items-center justify-between gap-3">
              <code class="break-all text-xs">{{ deletion.digest }}</code>
              <Badge :tone="deletion.status === 'completed' ? 'success' : 'danger'">{{ deletion.status }}</Badge>
            </div>
            <p class="mt-1 text-xs text-muted-foreground">Manual · {{ new Date(deletion.startedAt).toLocaleString() }} · {{ deletion.reason || 'No reason' }}</p>
          </Card>
          <PaginationControls
            :page="deletionPagination.page.value"
            :has-previous="deletionPagination.hasPrevious.value"
            :has-next="Boolean(artifactDeletionHistory.data.value?.nextCursor)"
            :disabled="artifactDeletionHistory.isFetching.value"
            @previous="deletionPagination.previous()"
            @next="deletionPagination.next(artifactDeletionHistory.data.value?.nextCursor)"
          />
          <Card v-for="run in pageItems(lifecycleHistory.data.value)" :key="run.id" class="p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs">Lifecycle · {{ run.items.length }} candidates</span>
              <Badge :tone="run.status === 'completed' ? 'success' : run.status === 'failed' ? 'danger' : 'warning'">{{ run.status }}</Badge>
            </div>
            <p class="mt-1 text-xs text-muted-foreground">{{ new Date(run.startedAt).toLocaleString() }} · {{ run.reason }}</p>
          </Card>
          <PaginationControls
            :page="lifecyclePagination.page.value"
            :has-previous="lifecyclePagination.hasPrevious.value"
            :has-next="Boolean(lifecycleHistory.data.value?.nextCursor)"
            :disabled="lifecycleHistory.isFetching.value"
            @previous="lifecyclePagination.previous()"
            @next="lifecyclePagination.next(lifecycleHistory.data.value?.nextCursor)"
          />
        </div>
      </section>
    </section>

    <section v-else-if="repositoryId && !repositories.isLoading.value" class="empty-state mt-5">
      <div>
        <p class="font-medium text-foreground">Repository not found</p>
        <RouterLink :to="{ name: 'project-detail', params: { project: slug } }" class="mt-2 inline-block text-sm text-accent">Return to project</RouterLink>
      </div>
    </section>

    <Dialog v-if="archiveRepositoryOpen && selectedRepository" labelled-by="archive-repository-title" @close="archiveRepositoryOpen = false">
      <form class="modal form-stack" aria-labelledby="archive-repository-title" @submit.prevent="archive.mutate()">
        <div class="flex items-start justify-between gap-4"><div><p class="eyebrow">Repository access</p><h2 id="archive-repository-title" class="text-lg font-semibold">Archive repository</h2></div><Button variant="ghost" size="icon" type="button" aria-label="Close repository archive confirmation" @click="archiveRepositoryOpen = false"><X :size="18" /></Button></div>
        <div class="deletion-warning"><AlertTriangle :size="18" /><p>Archive <strong>{{ selectedRepository.name }}</strong>? New image pushes will be blocked. Pulls and existing OCI content will remain available until you unarchive it.</p></div>
        <p v-if="repositoryOperationError" class="error-text" role="alert">{{ repositoryOperationError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" @click="archiveRepositoryOpen = false">Cancel</Button><Button type="submit" :disabled="archive.isPending.value">{{ archive.isPending.value ? 'Archiving…' : 'Archive repository' }}</Button></div>
      </form>
    </Dialog>

    <Dialog v-if="removeRepositoryOpen && selectedRepository" labelled-by="remove-repository-title" @close="removeRepositoryOpen = false">
      <form class="modal form-stack" aria-labelledby="remove-repository-title" @submit.prevent="removeLogicalRepository.mutate()">
        <div class="flex items-start justify-between gap-4"><div><p class="eyebrow">Destructive action</p><h2 id="remove-repository-title" class="text-lg font-semibold">Remove logical repository</h2></div><Button variant="ghost" size="icon" type="button" aria-label="Close logical repository removal" @click="removeRepositoryOpen = false"><X :size="18" /></Button></div>
        <div class="deletion-warning"><AlertTriangle :size="18" /><p>This removes Grom's archived repository record only. It never deletes OCI content; remove all manifests first.</p></div>
        <p v-if="repositoryOperationError" class="error-text" role="alert">{{ repositoryOperationError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" @click="removeRepositoryOpen = false">Cancel</Button><DeleteButton type="submit" :disabled="removeLogicalRepository.isPending.value">{{ removeLogicalRepository.isPending.value ? 'Removing…' : 'Remove record' }}</DeleteButton></div>
      </form>
    </Dialog>

    <Dialog v-if="selectedManifest" labelled-by="manifest-details-title" @close="selectedManifest = null">
      <section class="modal form-stack" aria-labelledby="manifest-details-title">
        <div class="flex items-start justify-between gap-4"><div><p class="eyebrow">Manifest</p><h2 id="manifest-details-title" class="text-lg font-semibold">Manifest details</h2></div><Button variant="ghost" size="icon" aria-label="Close manifest details" @click="selectedManifest = null"><X :size="18" /></Button></div>
        <div><p class="text-xs text-muted-foreground">Digest</p><code class="mt-1 block break-all text-xs">{{ selectedManifest.digest }}</code></div>
        <div class="grid grid-cols-2 gap-3 text-sm"><div><p class="text-xs text-muted-foreground">Media type</p><p class="break-all">{{ selectedManifest.mediaType || 'Unknown' }}</p></div><div><p class="text-xs text-muted-foreground">Manifest metadata</p><p>{{ formatCompressedSize(selectedManifest.manifestSize) }}</p></div><div><p class="text-xs text-muted-foreground">State</p><Badge :tone="manifestStateTone(selectedManifest.state)">{{ manifestStateLabel(selectedManifest.state) }}</Badge></div><div><p class="text-xs text-muted-foreground">Observed</p><p>{{ new Date(selectedManifest.firstSeenAt).toLocaleString() }}</p></div><div><p class="text-xs text-muted-foreground">Last seen</p><p>{{ new Date(selectedManifest.lastSeenAt).toLocaleString() }}</p></div></div>
        <div><p class="text-xs text-muted-foreground">Tags</p><div class="mt-1 flex flex-wrap gap-1"><Badge v-for="tag in manifestTags(selectedManifest)" :key="tag">{{ tag }}</Badge><span v-if="!manifestTags(selectedManifest).length" class="text-sm">{{ manifestStateLabel(selectedManifest.state) }}</span></div></div>
        <div class="grid grid-cols-2 gap-3 text-sm"><div><p class="text-xs text-muted-foreground">Classification</p><p>{{ selectedManifest.observedKind.replaceAll('_', ' ') }} · {{ selectedManifest.classificationConfidence }}</p></div><div><p class="text-xs text-muted-foreground">OCI relationship</p><p>{{ selectedManifest.artifactRelationship }}</p></div></div>
        <div v-if="selectedManifest.subjectDigest"><p class="text-xs text-muted-foreground">Subject digest</p><code class="mt-1 block break-all text-xs">{{ selectedManifest.subjectDigest }}</code></div>
      </section>
    </Dialog>

    <Dialog v-if="membershipToRemove" labelled-by="remove-member-title" @close="membershipToRemove = null">
      <form class="modal form-stack" aria-labelledby="remove-member-title" @submit.prevent="removeMember.mutate()">
        <div class="flex items-start justify-between gap-4"><div><p class="eyebrow">Access change</p><h2 id="remove-member-title" class="text-lg font-semibold">Remove member</h2></div><Button variant="ghost" size="icon" type="button" aria-label="Close member removal" @click="membershipToRemove = null"><X :size="18" /></Button></div>
        <div class="member-role-target">
          <div><p class="text-sm font-semibold">{{ membershipToRemove.principalName }}</p><p class="mt-1 text-xs text-muted-foreground">{{ membershipToRemove.principalDetail }}</p></div>
          <div class="flex items-center gap-2"><PrincipalTypeBadge :kind="membershipToRemove.principalKind" /><Badge>{{ membershipToRemove.role }}</Badge></div>
        </div>
        <div class="deletion-warning"><AlertTriangle :size="18" /><p>This member loses project access once their <strong>{{ membershipToRemove.role }}</strong> permission is removed. Access ends on the next registry token exchange.</p></div>
        <p v-if="memberError" class="error-text" role="alert">{{ memberError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" @click="membershipToRemove = null">Cancel</Button><DeleteButton type="submit" :disabled="removeMember.isPending.value">{{ removeMember.isPending.value ? 'Removing…' : 'Remove member' }}</DeleteButton></div>
      </form>
    </Dialog>

    <RepositoryCreateModal
      v-if="repositoryModal"
      :project="slug"
      @close="repositoryModal = false"
      @created="handleRepositoryCreated"
    />

    <Dialog v-if="pushInstructionsOpen" labelled-by="push-instructions-title" @close="pushInstructionsOpen = false">
      <section class="modal form-stack" aria-labelledby="push-instructions-title">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="eyebrow">Docker</p>
            <h2 id="push-instructions-title" class="text-lg font-semibold">Push instructions</h2>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close push instructions" @click="pushInstructionsOpen = false">
            <X :size="18" />
          </Button>
        </div>
        <DockerPushBanner :registry-host="registryHost" :project="slug" :repository="selectedRepository?.name" />
      </section>
    </Dialog>

    <Dialog v-if="deletionPreview" labelled-by="delete-artifact-title" :restore-focus="deletionTrigger" @close="deletionPreview = null">
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
        <div v-if="deletionPreview.childDigests.length">
          <p class="text-xs text-muted-foreground">Unreferenced platform manifests</p>
          <p class="mt-1 text-xs text-muted-foreground">These child manifests belong only to this image index and will be removed with it.</p>
          <code v-for="digest in deletionPreview.childDigests" :key="digest" class="mt-1 block break-all text-xs">{{ digest }}</code>
        </div>
        <label class="field-label">
          Reason <span v-if="deletionPreview.requiresReason">(required by policy)</span>
          <textarea v-model="deletionReason" class="field-control min-h-24 resize-y p-3" maxlength="500" />
        </label>
        <p v-if="deletionError" class="error-text">{{ deletionError }}</p>
        <div class="flex justify-end gap-2">
          <Button variant="ghost" type="button" @click="deletionPreview = null">Cancel</Button>
          <DeleteButton
            type="submit"
            :disabled="confirmDeletion.isPending.value || deletionPreview.blockedReasons.length > 0 || (deletionPreview.requiresReason && !deletionReason.trim())"
          >
            {{ confirmDeletion.isPending.value ? 'Deleting…' : 'Delete artifact' }}
          </DeleteButton>
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
          <div><strong>{{ lifecyclePreview.eligibleCount }}</strong> <span>eligible</span></div>
          <div><strong>{{ lifecyclePreview.retainedCount }}</strong> <span>retained</span></div>
          <div><strong>{{ lifecyclePreview.blockedCount }}</strong> <span>blocked</span></div>
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
            <DeleteButton
              :disabled="!lifecycleReason.trim() || !lifecyclePreview.eligibleCount || runLifecycle.isPending.value"
              @click="runLifecycle.mutate()"
            >
              {{ runLifecycle.isPending.value ? 'Executing…' : `Delete ${lifecyclePreview.eligibleCount} eligible` }}
            </DeleteButton>
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

    <Dialog v-if="memberToEdit && canManage" labelled-by="change-member-role-title" @close="closeChangeMemberRoleModal">
      <form class="modal form-stack" aria-labelledby="change-member-role-title" @submit.prevent="changeMemberRole.mutate()">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 id="change-member-role-title" class="text-lg font-semibold">Change role</h2>
            <p class="mt-1 text-sm text-muted-foreground">Update access for the selected member only.</p>
          </div>
          <Button variant="ghost" size="icon" type="button" aria-label="Close change role" @click="closeChangeMemberRoleModal"><X :size="18" /></Button>
        </div>
        <div class="member-role-target">
          <div><p class="text-sm font-semibold">{{ memberToEdit.principalName }}</p><p class="mt-1 text-xs text-muted-foreground">{{ memberToEdit.principalDetail }}</p></div>
          <PrincipalTypeBadge :kind="memberToEdit.principalKind" />
        </div>
        <label class="field-label">Role<select v-model="editedMemberRole" class="field-control text-sm"><option value="reader">Reader · pull</option><option value="writer">Writer · pull and push</option><option value="admin">Admin · manage project</option></select></label>
        <p v-if="memberError" class="error-text" role="alert">{{ memberError }}</p>
        <div class="flex justify-end gap-2"><Button variant="ghost" type="button" @click="closeChangeMemberRoleModal">Cancel</Button><Button type="submit" :disabled="changeMemberRole.isPending.value">{{ changeMemberRole.isPending.value ? 'Saving…' : 'Save role' }}</Button></div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.settings-members {
  border-top: 1px solid var(--border);
  padding-top: 1rem;
}

.project-settings-modal {
  width: min(100%, 48rem);
  max-height: calc(100vh - 2rem);
  overflow: auto;
}

.members-toolbar,
.members-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.members-toolbar {
  justify-content: space-between;
}

.member-search {
  position: relative;
  width: min(15rem, 34vw);
}

.member-search > svg {
  position: absolute;
  z-index: 1;
  top: 50%;
  left: 0.7rem;
  transform: translateY(-50%);
  color: var(--muted-foreground);
  pointer-events: none;
}

.member-search :deep(.grom-input) {
  padding-left: 2.15rem;
}

.member-table {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 0.7rem;
  background: var(--surface);
}

.member-tables {
  display: grid;
  gap: 1rem;
}

.member-table-group {
  display: grid;
  gap: 0.45rem;
}

.member-table-group-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
}

.member-table-group-heading h4 {
  margin: 0;
  font-size: 0.82rem;
}

.member-table-group-heading span {
  color: var(--muted-foreground);
  font-size: 0.72rem;
}

.member-role-target {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border: 1px solid var(--border);
  border-radius: 0.65rem;
  background: rgba(255, 255, 255, 0.018);
  padding: 0.8rem;
}

.member-table-head,
.member-row {
  display: grid;
  grid-template-columns: minmax(10rem, 1.25fr) minmax(7.75rem, 0.8fr) minmax(4.5rem, auto) minmax(6.5rem, 0.7fr) auto;
  align-items: center;
  gap: 0.8rem;
  padding: 0.75rem 1rem;
}

.member-table-head {
  border-bottom: 1px solid var(--border);
  color: var(--muted-foreground);
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.member-row + .member-row {
  border-top: 1px solid var(--border);
}

.member-role,
.member-type {
  justify-self: start;
}

.member-actions {
  justify-self: end;
}

@media (max-width: 700px) {
  .project-settings-modal {
    width: 100%;
    max-height: calc(100vh - 1.25rem);
    padding: 1rem;
  }

  .members-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .members-toolbar-actions {
    width: 100%;
  }

  .member-search {
    width: auto;
    flex: 1;
  }

  .member-table-head {
    display: none;
  }

  .member-row {
    grid-template-columns: minmax(0, 1fr) auto auto auto;
    gap: 0.55rem;
    padding: 0.7rem 0.85rem;
  }

  .member-identity {
    min-width: 0;
  }

  .member-identity p:first-child {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .member-identity p:last-child {
    display: none;
  }

  .member-type {
    display: inline-flex;
    grid-column: 2;
    grid-row: 1;
  }

  .member-role {
    grid-column: 3;
    grid-row: 1;
  }

  .member-assigned {
    display: none;
  }

  .member-actions {
    grid-column: 4;
    grid-row: 1;
    justify-self: end;
  }
}

.project-registry-url {
  margin: .28rem 0 0;
  color: var(--muted-foreground);
  font-size: .72rem;
  line-height: 1.35;
}

.repository-detail {
  max-width: 1240px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: .85rem;
}

.overview-card {
  display: flex;
  min-height: 8.5rem;
  gap: 1rem;
}

.overview-icon {
  display: grid;
  flex: 0 0 auto;
  width: 2.35rem;
  height: 2.35rem;
  place-items: center;
  border: 1px solid rgba(145, 173, 36, .16);
  border-radius: .7rem;
  background: rgba(145, 173, 36, .08);
  color: #c9df6c;
}

.overview-label { color: var(--muted-foreground); font-size: .72rem; font-weight: 600; }
.overview-value { margin-top: .3rem; font-size: 1.1rem; font-weight: 680; }
.overview-detail { margin-top: .5rem; color: var(--muted-foreground); font-size: .76rem; line-height: 1.4; }

.repo-panel {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: .8rem;
  background: var(--card);
}

.panel-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid var(--border);
  padding: 1.15rem 1.25rem;
}

.panel-heading h2 { font-weight: 650; }
.panel-heading p { margin-top: .25rem; color: var(--muted-foreground); font-size: .8rem; }

.repo-panel-body {
  display: grid;
  gap: .85rem;
  padding: 1.15rem 1.25rem;
}

.policy-summary-list {
  display: grid;
  gap: .6rem;
  list-style: none;
  padding: 0;
}

.policy-summary-item {
  border: 1px solid var(--border);
  border-radius: .65rem;
  padding: .65rem .75rem;
}

.policy-summary-item-enabled {
  border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 35%, transparent);
}

@media (max-width: 900px) {
  .overview-grid {
    grid-template-columns: 1fr;
  }
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

.operation-warning {
  display: flex;
  gap: .75rem;
  border: 1px solid color-mix(in srgb, var(--warning) 28%, transparent);
  border-radius: .75rem;
  background: color-mix(in srgb, var(--warning) 7%, transparent);
  padding: .9rem 1rem;
  color: var(--foreground);
}

.operation-warning p { margin-top: .2rem; color: var(--muted-foreground); font-size: .82rem; }

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
