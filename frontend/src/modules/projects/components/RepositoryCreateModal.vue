<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import type { PolicyPreset, RepositoryPolicyInput } from '@/shared/api/models'
import { Badge } from '@/shared/components/ui/badge'
import { Button } from '@/shared/components/ui/button'
import { Dialog } from '@/shared/components/ui/dialog'
import { Input } from '@/shared/components/ui/input'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { AlertTriangle, Check, ChevronDown, RotateCcw, ShieldCheck, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { createRepository, listPolicyPresets, projectKeys } from '../api/projects'

const props = defineProps<{ project: string }>()
const emit = defineEmits<{ close: []; created: [] }>()
const queryClient = useQueryClient()

const name = ref('')
const description = ref('')
const error = ref('')
const selectedKeys = ref<string[]>([])
const expandedKeys = ref<string[]>([])
const configurations = ref<Record<string, RepositoryPolicyInput>>({})

const presets = useQuery({ queryKey: projectKeys.policyPresets, queryFn: listPolicyPresets })
watch(() => presets.data.value, (availablePresets) => {
  for (const preset of availablePresets ?? []) {
    if (!configurations.value[preset.key]) {
      configurations.value[preset.key] = clonePolicy(preset)
    }
  }
}, { immediate: true })

const selectedPolicies = computed(() =>
  selectedKeys.value
    .map((key) => configurations.value[key])
    .filter((policy): policy is RepositoryPolicyInput => Boolean(policy)),
)

const create = useMutation({
  mutationFn: () => createRepository(props.project, {
    name: name.value,
    description: description.value,
    policies: selectedPolicies.value,
  }),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(props.project) })
    emit('created')
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create repository'
  },
})

function clonePolicy(preset: PolicyPreset): RepositoryPolicyInput {
  return {
    ...preset.policy,
    tagPatterns: preset.policy.tagPatterns ? [...preset.policy.tagPatterns] : undefined,
    allowedPatterns: preset.policy.allowedPatterns ? [...preset.policy.allowedPatterns] : undefined,
  }
}

function isSelected(key: string) {
  return selectedKeys.value.includes(key)
}

function isExpanded(key: string) {
  return expandedKeys.value.includes(key)
}

function toggleSelected(preset: PolicyPreset) {
  if (isSelected(preset.key)) {
    selectedKeys.value = selectedKeys.value.filter((key) => key !== preset.key)
    return
  }
  if (!configurations.value[preset.key]) {
    configurations.value[preset.key] = clonePolicy(preset)
  }
  selectedKeys.value = [...selectedKeys.value, preset.key]
  if (!isExpanded(preset.key)) {
    expandedKeys.value = [...expandedKeys.value, preset.key]
  }
}

function toggleExpanded(key: string) {
  expandedKeys.value = isExpanded(key)
    ? expandedKeys.value.filter((item) => item !== key)
    : [...expandedKeys.value, key]
}

function configurationFor(preset: PolicyPreset) {
  return configurations.value[preset.key] ?? preset.policy
}

function resetPolicy(preset: PolicyPreset) {
  configurations.value[preset.key] = clonePolicy(preset)
}

function updatePatterns(preset: PolicyPreset, field: 'tagPatterns' | 'allowedPatterns', raw: string) {
  const policy = ensureConfiguration(preset)
  policy[field] = raw.split(',').map((value) => value.trim()).filter(Boolean)
}

function updateNumber(
  preset: PolicyPreset,
  field: 'expireAfterDays' | 'keepLast' | 'untaggedGraceDays',
  raw: string,
) {
  const parsed = Number.parseInt(raw, 10)
  ensureConfiguration(preset)[field] = Number.isFinite(parsed) ? parsed : undefined
}

function updateBoolean(
  preset: PolicyPreset,
  field: 'preventOverwrite' | 'preventDeletion' | 'excludeFromLifecycle' | 'requireReason',
  event: unknown,
) {
  ensureConfiguration(preset)[field] = (event as { target: { checked: boolean } }).target.checked
}

function ensureConfiguration(preset: PolicyPreset): RepositoryPolicyInput {
  const existing = configurations.value[preset.key]
  if (existing) {
    return existing
  }
  const created = clonePolicy(preset)
  configurations.value[preset.key] = created
  return created
}

function summary(preset: PolicyPreset) {
  const policy = configurationFor(preset)
  switch (policy.type) {
    case 'tag_protection':
      return `Protect ${policy.tagPatterns?.join(', ') || 'selected tags'}`
    case 'immutability':
      return `${policy.tagPatterns?.join(', ') || 'Selected tags'} become immutable`
    case 'retention':
      return `Expire after ${policy.expireAfterDays ?? '—'} days · keep ${policy.keepLast ?? '—'}`
    case 'tag_naming':
      return `Accept ${policy.allowedPatterns?.join(', ') || 'configured patterns'}`
    case 'manual_deletion':
      return policy.requireReason ? 'Deletion reason required' : 'Standard confirmation'
  }
}
</script>

<template>
  <Dialog labelled-by="create-repository-title" @close="emit('close')">
    <form class="repository-modal" aria-labelledby="create-repository-title" @submit.prevent="create.mutate()">
      <header class="repository-modal-header">
        <div>
          <p class="eyebrow">Repository setup</p>
          <h2 id="create-repository-title" class="text-xl font-semibold">Create repository</h2>
          <p class="mt-1 text-sm text-muted-foreground">Choose the behavior before the first image is pushed.</p>
        </div>
        <Button variant="ghost" size="icon" type="button" aria-label="Close" @click="emit('close')">
          <X :size="18" />
        </Button>
      </header>

      <div class="repository-form-section">
        <div class="section-heading">
          <span class="step-number">1</span>
          <div>
            <h3>Repository details</h3>
            <p>The path is relative to {{ project }}/ and supports nested segments.</p>
          </div>
        </div>
        <div class="details-grid">
          <label class="field-label">
            Path
            <Input v-model="name" required placeholder="backend or services/api" />
          </label>
          <label class="field-label">
            Description
            <Input v-model="description" placeholder="Primary API image" />
          </label>
        </div>
      </div>

      <div class="repository-form-section">
        <div class="section-heading">
          <span class="step-number">2</span>
          <div>
            <h3>Behavior policies</h3>
            <p>Expand any policy to understand it. Selected values remain fully editable.</p>
          </div>
          <Badge class="selection-count">{{ selectedKeys.length }} selected</Badge>
        </div>

        <div v-if="presets.isLoading.value" class="policy-loading">Loading recommended policies…</div>
        <p v-else-if="presets.isError.value" class="error-text">Could not load policy recommendations.</p>
        <div v-else class="policy-list">
          <article
            v-for="preset in presets.data.value"
            :key="preset.key"
            class="policy-accordion"
            :class="{ selected: isSelected(preset.key) }"
          >
            <div class="policy-accordion-header">
              <label class="policy-selector">
                <input
                  type="checkbox"
                  :checked="isSelected(preset.key)"
                  :aria-label="`Select ${preset.name}`"
                  @change="toggleSelected(preset)"
                />
                <span class="checkbox-visual"><Check :size="13" /></span>
              </label>
              <button
                type="button"
                class="policy-toggle"
                :aria-expanded="isExpanded(preset.key)"
                :aria-controls="`policy-${preset.key}`"
                @click="toggleExpanded(preset.key)"
              >
                <span class="policy-icon"><ShieldCheck :size="17" /></span>
                <span class="policy-title">
                  <span class="policy-name-line">
                    <strong>{{ preset.name }}</strong>
                    <Badge>{{ preset.category }}</Badge>
                  </span>
                  <span>{{ summary(preset) }}</span>
                </span>
                <ChevronDown class="policy-chevron" :class="{ open: isExpanded(preset.key) }" :size="18" />
              </button>
            </div>

            <div v-show="isExpanded(preset.key)" :id="`policy-${preset.key}`" class="policy-content">
              <div class="policy-explanation">
                <p>{{ preset.description }}</p>
                <div class="policy-outcome"><Check :size="15" /><span>{{ preset.outcome }}</span></div>
                <div v-if="preset.caution" class="policy-caution">
                  <AlertTriangle :size="15" /><span>{{ preset.caution }}</span>
                </div>
              </div>

              <fieldset :disabled="!isSelected(preset.key)" class="policy-fields">
                <label
                  v-if="configurationFor(preset).tagPatterns"
                  class="field-label"
                >
                  Tag patterns
                  <Input
                    :model-value="configurationFor(preset).tagPatterns?.join(', ')"
                    placeholder="stable, prod, v*"
                    @update:model-value="updatePatterns(preset, 'tagPatterns', $event)"
                  />
                  <small>Separate patterns with commas. Use * as a wildcard.</small>
                </label>

                <label
                  v-if="configurationFor(preset).allowedPatterns"
                  class="field-label"
                >
                  Allowed tag patterns
                  <Input
                    :model-value="configurationFor(preset).allowedPatterns?.join(', ')"
                    placeholder="v*, latest, sha-*"
                    @update:model-value="updatePatterns(preset, 'allowedPatterns', $event)"
                  />
                </label>

                <div v-if="configurationFor(preset).type === 'retention'" class="number-grid">
                  <label class="field-label">
                    Expire after days
                    <Input
                      type="number"
                      :model-value="String(configurationFor(preset).expireAfterDays ?? '')"
                      min="1"
                      max="3650"
                      @update:model-value="updateNumber(preset, 'expireAfterDays', $event)"
                    />
                  </label>
                  <label class="field-label">
                    Keep latest
                    <Input
                      type="number"
                      :model-value="String(configurationFor(preset).keepLast ?? '')"
                      min="1"
                      max="10000"
                      @update:model-value="updateNumber(preset, 'keepLast', $event)"
                    />
                  </label>
                  <label class="field-label">
                    Untagged grace
                    <Input
                      type="number"
                      :model-value="String(configurationFor(preset).untaggedGraceDays ?? '')"
                      min="1"
                      max="3650"
                      @update:model-value="updateNumber(preset, 'untaggedGraceDays', $event)"
                    />
                  </label>
                </div>

                <div class="toggle-grid">
                  <label
                    v-if="configurationFor(preset).type === 'tag_protection' || configurationFor(preset).type === 'immutability'"
                    class="toggle-field"
                  >
                    <input
                      :checked="configurationFor(preset).preventOverwrite === true"
                      type="checkbox"
                      @change="updateBoolean(preset, 'preventOverwrite', $event)"
                    />
                    Prevent overwrite
                  </label>
                  <label v-if="configurationFor(preset).type === 'tag_protection'" class="toggle-field">
                    <input
                      :checked="configurationFor(preset).preventDeletion === true"
                      type="checkbox"
                      @change="updateBoolean(preset, 'preventDeletion', $event)"
                    />
                    Prevent deletion
                  </label>
                  <label v-if="configurationFor(preset).type === 'tag_protection'" class="toggle-field">
                    <input
                      :checked="configurationFor(preset).excludeFromLifecycle === true"
                      type="checkbox"
                      @change="updateBoolean(preset, 'excludeFromLifecycle', $event)"
                    />
                    Exclude from lifecycle
                  </label>
                  <label v-if="configurationFor(preset).type === 'manual_deletion'" class="toggle-field">
                    <input
                      :checked="configurationFor(preset).requireReason === true"
                      type="checkbox"
                      @change="updateBoolean(preset, 'requireReason', $event)"
                    />
                    Require a reason
                  </label>
                </div>

                <Button variant="ghost" size="sm" type="button" class="reset-button" @click="resetPolicy(preset)">
                  <RotateCcw :size="14" /> Restore recommended values
                </Button>
              </fieldset>
            </div>
          </article>
        </div>
      </div>

      <p v-if="error" class="error-text">{{ error }}</p>
      <footer class="repository-modal-footer">
        <p>{{ project }}/<strong>{{ name || 'repository' }}</strong> · {{ selectedKeys.length }} policies</p>
        <div class="flex gap-2">
          <Button variant="ghost" type="button" @click="emit('close')">Cancel</Button>
          <Button type="submit" :disabled="!name || create.isPending.value">
            {{ create.isPending.value ? 'Creating…' : 'Create repository' }}
          </Button>
        </div>
      </footer>
    </form>
  </Dialog>
</template>

<style scoped>
.repository-modal {
  width: min(100%, 52rem);
  max-height: calc(100vh - 2rem);
  overflow: auto;
  border: 1px solid var(--border-strong);
  border-radius: 1rem;
  background: var(--surface);
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.58);
}

.repository-modal-header,
.repository-modal-footer {
  position: sticky;
  z-index: 2;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  background: color-mix(in srgb, var(--surface) 95%, transparent);
  padding: 1.25rem 1.4rem;
  backdrop-filter: blur(12px);
}

.repository-modal-header {
  top: 0;
  border-bottom: 1px solid var(--border);
}

.repository-modal-footer {
  bottom: 0;
  align-items: center;
  border-top: 1px solid var(--border);
}

.repository-modal-footer p {
  margin: 0;
  color: var(--muted-foreground);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: .75rem;
}

.repository-form-section {
  display: grid;
  gap: 1rem;
  padding: 1.35rem 1.4rem;
  border-bottom: 1px solid var(--border);
}

.section-heading {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: start;
  gap: .75rem;
}

.section-heading h3 {
  margin: 0;
  font-size: .95rem;
}

.section-heading p {
  margin: .25rem 0 0;
  color: var(--muted-foreground);
  font-size: .78rem;
  line-height: 1.5;
}

.step-number {
  display: grid;
  width: 1.55rem;
  height: 1.55rem;
  place-items: center;
  border: 1px solid rgba(145, 173, 36, .22);
  border-radius: .45rem;
  background: rgba(145, 173, 36, .08);
  color: #c9df6c;
  font-size: .7rem;
  font-weight: 700;
}

.selection-count {
  margin-top: .1rem;
}

.details-grid,
.number-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: .8rem;
}

.number-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.policy-list {
  display: grid;
  gap: .65rem;
}

.policy-accordion {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: .75rem;
  background: rgba(0, 0, 0, .1);
  transition: border-color var(--motion-standard) ease, background var(--motion-standard) ease;
}

.policy-accordion.selected {
  border-color: rgba(145, 173, 36, .28);
  background: rgba(145, 173, 36, .035);
}

.policy-accordion-header {
  display: flex;
  align-items: stretch;
}

.policy-selector {
  display: grid;
  width: 3.1rem;
  flex: none;
  place-items: center;
  cursor: pointer;
}

.policy-selector input {
  position: absolute;
  opacity: 0;
}

.checkbox-visual {
  display: grid;
  width: 1.15rem;
  height: 1.15rem;
  place-items: center;
  border: 1px solid var(--border-strong);
  border-radius: .3rem;
  color: transparent;
}

.policy-selector input:focus-visible + .checkbox-visual {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.policy-selector input:checked + .checkbox-visual {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--accent-foreground);
}

.policy-toggle {
  display: grid;
  width: 100%;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: .75rem;
  border: 0;
  background: transparent;
  padding: .85rem 1rem .85rem 0;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.policy-toggle:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: -3px;
}

.policy-icon {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  border-radius: .55rem;
  background: var(--muted);
  color: #c9df6c;
}

.policy-title {
  display: grid;
  gap: .3rem;
  color: var(--muted-foreground);
  font-size: .75rem;
}

.policy-name-line {
  display: flex;
  align-items: center;
  gap: .55rem;
  color: var(--foreground);
  font-size: .84rem;
}

.policy-chevron {
  color: var(--muted-foreground);
  transition: transform var(--motion-standard) ease;
}

.policy-chevron.open {
  transform: rotate(180deg);
}

.policy-content {
  display: grid;
  grid-template-columns: minmax(0, .8fr) minmax(0, 1.2fr);
  gap: 1rem;
  border-top: 1px solid var(--border);
  padding: 1rem;
}

.policy-explanation {
  color: var(--muted-foreground);
  font-size: .78rem;
  line-height: 1.55;
}

.policy-explanation > p {
  margin: 0 0 .8rem;
}

.policy-outcome,
.policy-caution {
  display: flex;
  gap: .5rem;
  border-radius: .55rem;
  padding: .6rem;
}

.policy-outcome {
  background: rgba(145, 173, 36, .07);
  color: #cadf75;
}

.policy-caution {
  margin-top: .5rem;
  background: rgba(211, 166, 82, .07);
  color: #e0bd7a;
}

.policy-fields {
  display: grid;
  gap: .8rem;
  min-width: 0;
  margin: 0;
  border: 0;
  padding: 0;
}

.policy-fields:disabled {
  opacity: .52;
}

.field-label small {
  color: var(--muted-foreground);
  font-weight: 400;
}

.toggle-grid {
  display: flex;
  flex-wrap: wrap;
  gap: .5rem .9rem;
}

.toggle-field {
  display: inline-flex;
  align-items: center;
  gap: .45rem;
  color: #d2ccc2;
  font-size: .76rem;
}

.toggle-field input {
  accent-color: var(--accent);
}

.reset-button {
  justify-self: start;
}

.policy-loading {
  border: 1px dashed var(--border);
  border-radius: .75rem;
  padding: 2rem;
  color: var(--muted-foreground);
  text-align: center;
}

@media (max-width: 700px) {
  .repository-modal {
    max-height: calc(100dvh - 1rem);
  }

  .details-grid,
  .number-grid,
  .policy-content {
    grid-template-columns: 1fr;
  }

  .section-heading {
    grid-template-columns: auto 1fr;
  }

  .selection-count {
    grid-column: 2;
    justify-self: start;
  }

  .repository-modal-footer {
    align-items: stretch;
    flex-direction: column;
  }

  .repository-modal-footer > div,
  .repository-modal-footer button {
    flex: 1;
  }
}
</style>
