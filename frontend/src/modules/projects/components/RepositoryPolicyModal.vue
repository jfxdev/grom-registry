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
import { ComboboxSelect } from '@/shared/components/ui/combobox'
import { Dialog } from '@/shared/components/ui/dialog'
import { Plus, Trash2, X } from '@lucide/vue'
import { ref } from 'vue'
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
    keepLast: policy.keepLast,
    untaggedGraceDays: policy.untaggedGraceDays,
    allowedPatterns: policy.allowedPatterns ? [...policy.allowedPatterns] : undefined,
    requireReason: policy.requireReason,
  }
}

const policies = ref<RepositoryPolicyInput[]>(props.repository.policies.map(toInput))
const newType = ref<RepositoryPolicyType>('retention')
const saving = ref(false)
const error = ref('')

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
    policy.keepLast = 10
    policy.untaggedGraceDays = 7
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

async function save() {
  saving.value = true
  error.value = ''
  try {
    const policySet = await replaceRepositoryPolicies(props.project, props.repository.id, {
      expectedVersion: props.repository.policyVersion,
      policies: policies.value,
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
  <Dialog labelled-by="repository-policies-title" @close="emit('close')">
    <section class="modal policy-modal form-stack">
      <div class="flex items-start justify-between gap-4">
        <div>
          <p class="eyebrow">Repository behavior</p>
          <h2 id="repository-policies-title" class="text-lg font-semibold">Policies for {{ repository.name }}</h2>
          <p class="mt-1 text-xs text-muted-foreground">Version {{ repository.policyVersion }}</p>
        </div>
        <Button variant="ghost" size="icon" aria-label="Close policies" @click="emit('close')">
          <X :size="18" />
        </Button>
      </div>

      <div v-if="!policies.length" class="rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">
        This repository has no active behavior policies.
      </div>

      <div class="policy-list">
        <Card v-for="(policy, index) in policies" :key="index" class="policy-card">
          <div class="flex items-center justify-between gap-3">
            <strong>{{ policyTypes.find((item) => item.value === policy.type)?.label }}</strong>
            <div class="flex items-center gap-3">
              <label class="toggle-field">
                <input v-model="policy.enabled" type="checkbox" />
                Enabled
              </label>
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

          <div v-if="policy.type === 'retention'" class="number-grid">
            <label class="field-label">Expire after days
              <input class="field-control" type="number" min="1" max="3650" :value="policy.expireAfterDays ?? ''" @input="setOptionalNumber(policy, 'expireAfterDays', inputValue($event))" />
            </label>
            <label class="field-label">Keep last
              <input class="field-control" type="number" min="1" max="10000" :value="policy.keepLast ?? ''" @input="setOptionalNumber(policy, 'keepLast', inputValue($event))" />
            </label>
            <label class="field-label">Untagged grace days
              <input class="field-control" type="number" min="1" max="3650" :value="policy.untaggedGraceDays ?? ''" @input="setOptionalNumber(policy, 'untaggedGraceDays', inputValue($event))" />
            </label>
          </div>

          <div v-if="policy.type === 'tag_protection'" class="toggle-grid">
            <label class="toggle-field"><input v-model="policy.preventDeletion" type="checkbox" /> Prevent deletion</label>
            <label class="toggle-field"><input v-model="policy.preventOverwrite" type="checkbox" /> Prevent overwrite</label>
            <label class="toggle-field"><input v-model="policy.excludeFromLifecycle" type="checkbox" /> Exclude from lifecycle</label>
          </div>

          <label v-if="policy.type === 'immutability'" class="toggle-field">
            <input v-model="policy.preventOverwrite" type="checkbox" />
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
            <input v-model="policy.requireReason" type="checkbox" />
            Require a deletion reason
          </label>
        </Card>
      </div>

      <div class="add-policy">
        <ComboboxSelect v-model="newType" :options="policyTypes" placeholder="Select a policy type…" empty-text="No matching policy type." />
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
}

.number-grid,
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

@media (max-width: 640px) {
  .number-grid,
  .toggle-grid {
    grid-template-columns: 1fr;
  }
}
</style>
