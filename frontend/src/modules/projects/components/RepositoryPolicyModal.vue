<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import type {
  Repository,
  RepositoryPolicyInput,
  RepositoryPolicySet,
  RepositoryPolicyType,
} from '@/shared/api/models'
import { Button } from '@/shared/components/ui/button'
import { Card } from '@/shared/components/ui/card'
import { Dialog } from '@/shared/components/ui/dialog'
import { Select } from '@/shared/components/ui/select'
import { Plus, Trash2, X } from '@lucide/vue'
import { nextTick, onMounted, ref } from 'vue'
import { replaceRepositoryPolicies } from '../api/projects'

const props = defineProps<{ project: string; repository: Repository }>()
const emit = defineEmits<{
  close: []
  saved: [policySet: RepositoryPolicySet]
}>()

const policyTypes: Array<{ value: RepositoryPolicyType; label: string }> = [
  { value: 'retention', label: 'Retention' },
  { value: 'tag_protection', label: 'Tag protection' },
  { value: 'immutability', label: 'Immutability' },
  { value: 'tag_naming', label: 'Tag naming' },
  { value: 'manual_deletion', label: 'Manual deletion' },
]

function toInput(policy: Repository['policies'][number]): RepositoryPolicyInput {
  return {
    type: policy.type,
    enabled: policy.enabled,
    tagPatterns: policy.tagPatterns ? [...policy.tagPatterns] : undefined,
    preventOverwrite: policy.preventOverwrite,
    preventDeletion: policy.preventDeletion,
    excludeFromLifecycle: policy.excludeFromLifecycle,
    expireAfterDays: policy.expireAfterDays,
    expireAfterDaysEnabled: retentionCriterionEnabled(policy, 'expireAfterDaysEnabled', 'expireAfterDays'),
    keepLast: policy.keepLast,
    keepLastEnabled: retentionCriterionEnabled(policy, 'keepLastEnabled', 'keepLast'),
    untaggedGraceDays: policy.untaggedGraceDays,
    untaggedGraceDaysEnabled: retentionCriterionEnabled(policy, 'untaggedGraceDaysEnabled', 'untaggedGraceDays'),
    allowedPatterns: policy.allowedPatterns ? [...policy.allowedPatterns] : undefined,
    requireReason: policy.requireReason,
  }
}

const policies = ref<RepositoryPolicyInput[]>(props.repository.policies.map(toInput))
const newType = ref<RepositoryPolicyType>('retention')
const saving = ref(false)
const error = ref('')
const dialogRoot = ref<{ element?: globalThis.HTMLElement }>()
const selectPortal = ref<globalThis.HTMLElement>()

onMounted(async () => {
  await nextTick()
  selectPortal.value = dialogRoot.value?.element
})

function addPolicy() {
  const policy: RepositoryPolicyInput = {
    type: newType.value,
    enabled: true,
    preventOverwrite: false,
    preventDeletion: false,
    excludeFromLifecycle: false,
    requireReason: false,
  }
  if (newType.value === 'retention') {
    policy.expireAfterDays = 30
    policy.expireAfterDaysEnabled = true
    policy.keepLast = 10
    policy.keepLastEnabled = true
    policy.untaggedGraceDays = 7
    policy.untaggedGraceDaysEnabled = true
    policy.tagPatterns = ['*']
  } else if (newType.value === 'tag_protection') {
    policy.tagPatterns = ['prod', 'v*']
    policy.preventDeletion = true
    policy.excludeFromLifecycle = true
  } else if (newType.value === 'immutability') {
    policy.tagPatterns = ['prod', 'v*']
    policy.preventOverwrite = true
  } else if (newType.value === 'tag_naming') {
    policy.allowedPatterns = ['latest', 'v*']
  } else {
    policy.requireReason = true
  }
  policies.value.push(policy)
}

function patterns(policy: RepositoryPolicyInput, field: 'tagPatterns' | 'allowedPatterns') {
  return policy[field]?.join(', ') ?? ''
}

function setPatterns(policy: RepositoryPolicyInput, field: 'tagPatterns' | 'allowedPatterns', raw: string) {
  policy[field] = raw.split(',').map((value) => value.trim()).filter(Boolean)
}

function inputValue(event: unknown) {
  return (event as { target: { value: string } }).target.value
}

function setOptionalNumber(
  policy: RepositoryPolicyInput,
  field: 'expireAfterDays' | 'keepLast' | 'untaggedGraceDays',
  raw: string,
) {
  const parsed = Number(raw)
  policy[field] = Number.isFinite(parsed) && Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
}

type RetentionEnabledField = 'expireAfterDaysEnabled' | 'keepLastEnabled' | 'untaggedGraceDaysEnabled'
type RetentionLimitField = 'expireAfterDays' | 'keepLast' | 'untaggedGraceDays'

function retentionCriterionEnabled(
  policy: RepositoryPolicyInput,
  enabledField: RetentionEnabledField,
  limitField: RetentionLimitField,
) {
  return policy[enabledField] ?? policy[limitField] !== undefined
}

function setRetentionCriterionEnabled(policy: RepositoryPolicyInput, field: RetentionEnabledField, event: globalThis.Event) {
  policy[field] = event.target instanceof globalThis.HTMLInputElement && event.target.checked
}

function toPayload(policy: RepositoryPolicyInput): RepositoryPolicyInput {
  if (policy.type !== 'retention' || policy.enabled) return policy
  return {
    ...policy,
    expireAfterDays: undefined,
    expireAfterDaysEnabled: undefined,
    keepLast: undefined,
    keepLastEnabled: undefined,
    untaggedGraceDays: undefined,
    untaggedGraceDaysEnabled: undefined,
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const policySet = await replaceRepositoryPolicies(props.project, props.repository.id, {
      expectedVersion: props.repository.policyVersion,
      policies: policies.value.map(toPayload),
    })
    emit('saved', policySet)
  } catch (caught) {
    error.value = caught instanceof APIError ? caught.message : 'Could not save repository policies'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog ref="dialogRoot" labelled-by="repository-policies-title" @close="emit('close')">
    <section class="modal policy-modal form-stack">
      <div class="flex items-start justify-between gap-4">
        <div>
          <p class="eyebrow">Repository behavior</p>
          <h2 id="repository-policies-title" class="text-lg font-semibold">Policies for {{ repository.name }}</h2>
          <p class="mt-1 text-xs text-muted-foreground">Version {{ repository.policyVersion }} · Patterns use glob syntax (e.g. prod, v*, pr-*).</p>
        </div>
        <Button variant="ghost" size="icon" aria-label="Close policies" @click="emit('close')">
          <X :size="18" />
        </Button>
      </div>

      <div v-if="!policies.length" class="rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">
        This repository has no active behavior policies.
      </div>

      <div class="policy-list">
        <Card v-for="(policy, index) in policies" :key="index" class="policy-card" :class="{ 'policy-card-enabled': policy.enabled }">
          <div class="flex items-center justify-between gap-3">
            <strong>{{ policyTypes.find((item) => item.value === policy.type)?.label }}</strong>
            <div class="flex items-center gap-3">
              <input v-model="policy.enabled" class="retention-toggle" type="checkbox" :aria-label="`Enable ${policyTypes.find((item) => item.value === policy.type)?.label} policy`" />
              <Button variant="ghost" size="icon" :aria-label="`Remove ${policy.type} policy`" @click="policies.splice(index, 1)">
                <Trash2 :size="15" />
              </Button>
            </div>
          </div>

          <label v-if="['retention', 'tag_protection', 'immutability'].includes(policy.type)" class="field-label">
            Tag patterns
            <input
              class="field-control"
              :value="patterns(policy, 'tagPatterns')"
              placeholder="prod, v*, pr-*"
              @input="setPatterns(policy, 'tagPatterns', inputValue($event))"
            />
          </label>

          <div v-if="policy.type === 'retention'" class="retention-criteria">
            <article class="retention-criterion-card" :class="{ disabled: !retentionCriterionEnabled(policy, 'expireAfterDaysEnabled', 'expireAfterDays') }">
              <div class="retention-criterion">
                <strong>Expire after days</strong>
                <small>Remove matching images after this age.</small>
                <div class="retention-control">
                  <input class="field-control" type="number" min="1" max="3650" :disabled="!retentionCriterionEnabled(policy, 'expireAfterDaysEnabled', 'expireAfterDays')" :value="policy.expireAfterDays ?? ''" @input="setOptionalNumber(policy, 'expireAfterDays', inputValue($event))" />
                  <input class="retention-toggle" :checked="retentionCriterionEnabled(policy, 'expireAfterDaysEnabled', 'expireAfterDays')" type="checkbox" aria-label="Enable expire after days" @change="setRetentionCriterionEnabled(policy, 'expireAfterDaysEnabled', $event)" />
                </div>
              </div>
            </article>
            <article class="retention-criterion-card" :class="{ disabled: !retentionCriterionEnabled(policy, 'keepLastEnabled', 'keepLast') }">
              <div class="retention-criterion">
                <strong>Keep last</strong>
                <small>Always retain this many of the newest images.</small>
                <div class="retention-control">
                  <input class="field-control" type="number" min="1" max="10000" :disabled="!retentionCriterionEnabled(policy, 'keepLastEnabled', 'keepLast')" :value="policy.keepLast ?? ''" @input="setOptionalNumber(policy, 'keepLast', inputValue($event))" />
                  <input class="retention-toggle" :checked="retentionCriterionEnabled(policy, 'keepLastEnabled', 'keepLast')" type="checkbox" aria-label="Enable keep last" @change="setRetentionCriterionEnabled(policy, 'keepLastEnabled', $event)" />
                </div>
              </div>
            </article>
            <article class="retention-criterion-card" :class="{ disabled: !retentionCriterionEnabled(policy, 'untaggedGraceDaysEnabled', 'untaggedGraceDays') }">
              <div class="retention-criterion">
                <strong>Untagged grace days</strong>
                <small>Clean untagged images after this grace period.</small>
                <div class="retention-control">
                  <input class="field-control" type="number" min="1" max="3650" :disabled="!retentionCriterionEnabled(policy, 'untaggedGraceDaysEnabled', 'untaggedGraceDays')" :value="policy.untaggedGraceDays ?? ''" @input="setOptionalNumber(policy, 'untaggedGraceDays', inputValue($event))" />
                  <input class="retention-toggle" :checked="retentionCriterionEnabled(policy, 'untaggedGraceDaysEnabled', 'untaggedGraceDays')" type="checkbox" aria-label="Enable untagged grace days" @change="setRetentionCriterionEnabled(policy, 'untaggedGraceDaysEnabled', $event)" />
                </div>
              </div>
            </article>
          </div>

          <div v-if="policy.type === 'tag_protection'" class="toggle-grid">
            <label class="toggle-field"><input v-model="policy.preventDeletion" class="retention-toggle" type="checkbox" /> Prevent deletion</label>
            <label class="toggle-field"><input v-model="policy.preventOverwrite" class="retention-toggle" type="checkbox" /> Prevent overwrite</label>
            <label class="toggle-field"><input v-model="policy.excludeFromLifecycle" class="retention-toggle" type="checkbox" /> Exclude from lifecycle</label>
          </div>

          <label v-if="policy.type === 'immutability'" class="toggle-field">
            <input v-model="policy.preventOverwrite" class="retention-toggle" type="checkbox" />
            Prevent tag overwrite
          </label>

          <label v-if="policy.type === 'tag_naming'" class="field-label">
            Allowed patterns
            <input
              class="field-control"
              :value="patterns(policy, 'allowedPatterns')"
              placeholder="latest, main, v*"
              @input="setPatterns(policy, 'allowedPatterns', inputValue($event))"
            />
          </label>

          <label v-if="policy.type === 'manual_deletion'" class="toggle-field">
            <input v-model="policy.requireReason" class="retention-toggle" type="checkbox" />
            Require a deletion reason
          </label>
        </Card>
      </div>

      <div class="add-policy">
        <!-- eslint-disable-next-line vue/attribute-hyphenation -- vue-tsc does not resolve kebab-case attrs to camelCase props on generic components -->
        <Select v-model="newType" :options="policyTypes" ariaLabel="Select policy type" :portal-to="selectPortal" class="policy-type-select" />
        <Button variant="outline" @click="addPolicy"><Plus :size="15" /> Add policy</Button>
      </div>

      <p v-if="error" class="error-text">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="emit('close')">Cancel</Button>
        <Button :disabled="saving" @click="save">{{ saving ? 'Saving…' : 'Save policies' }}</Button>
      </div>
    </section>
  </Dialog>
</template>

<style scoped>
.policy-modal {
  width: min(760px, calc(100vw - 2rem));
  max-height: min(88vh, 900px);
  overflow: auto;
}

.policy-list {
  display: grid;
  gap: .75rem;
}

.policy-card {
  display: grid;
  gap: .85rem;
  padding: 1rem;
  transition: border-color var(--motion-standard) ease, box-shadow var(--motion-standard) ease;
}

.policy-card-enabled {
  border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 35%, transparent), 0 0 16px color-mix(in srgb, var(--accent) 30%, transparent);
}

.retention-criteria,
.toggle-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: .75rem;
}

.add-policy {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: .75rem;
}

.add-policy :deep(.policy-type-select) {
  width: 100%;
}

.retention-criterion {
  display: grid;
  gap: .45rem;
}

.retention-control {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: .65rem;
}

.retention-toggle {
  width: 1rem;
  height: 1rem;
  accent-color: var(--accent);
}

.retention-criterion-card {
  display: grid;
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: .75rem;
  background: color-mix(in srgb, var(--surface-raised) 72%, transparent);
  padding: .8rem;
  transition: opacity var(--motion-standard) ease, border-color var(--motion-standard) ease;
}

.retention-criterion-card.disabled {
  border-color: color-mix(in srgb, var(--border) 70%, transparent);
  opacity: .55;
}

@media (max-width: 640px) {
  .retention-criteria,
  .toggle-grid {
    grid-template-columns: 1fr;
  }
}
</style>
