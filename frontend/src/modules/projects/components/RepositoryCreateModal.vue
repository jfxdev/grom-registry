<script setup lang="ts">
import { APIError } from '@/shared/api/client'
import { Button, CancelButton } from '@/shared/components/ui/button'
import { Dialog } from '@/shared/components/ui/dialog'
import { Input } from '@/shared/components/ui/input'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { X } from '@lucide/vue'
import { ref } from 'vue'
import { createRepository, projectKeys } from '../api/projects'

const props = defineProps<{ project: string }>()
const emit = defineEmits<{ close: []; created: [] }>()
const queryClient = useQueryClient()

const name = ref('')
const description = ref('')
const error = ref('')

const create = useMutation({
  mutationFn: () => createRepository(props.project, {
    name: name.value,
    description: description.value,
    policies: [],
  }),
  onSuccess: async () => {
    await queryClient.invalidateQueries({ queryKey: projectKeys.repositories(props.project) })
    emit('created')
  },
  onError: (caught) => {
    error.value = caught instanceof APIError ? caught.message : 'Could not create repository'
  },
})
</script>

<template>
  <Dialog labelled-by="create-repository-title" @close="emit('close')">
    <form class="repository-modal" aria-labelledby="create-repository-title" @submit.prevent="create.mutate()">
      <header class="repository-modal-header">
        <div>
          <p class="eyebrow">Repository setup</p>
          <h2 id="create-repository-title" class="text-xl font-semibold">Create repository</h2>
          <p class="mt-1 text-sm text-muted-foreground">Policies can be configured after the repository is created.</p>
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
        <div class="path-examples">
          <p>Examples:</p>
          <ul>
            <li><code>backend</code></li>
            <li><code>base-images/forgejo</code></li>
            <li><code>services/api</code></li>
          </ul>
        </div>
      </div>

      <p v-if="error" class="error-text">{{ error }}</p>
      <footer class="repository-modal-footer">
        <p>{{ project }}/<strong>{{ name || 'repository' }}</strong></p>
        <div class="flex gap-2">
          <CancelButton @click="emit('close')" />
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
  box-shadow: 0 24px 80px rgba(0, 0, 0, .58);
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
}

.section-heading {
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: start;
  gap: .75rem;
}

.section-heading h3 {
  margin: 0;
  font-size: .95rem;
}

.section-heading p,
.path-examples p {
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

.details-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: .8rem;
}

.path-examples {
  margin: 0;
  color: var(--muted-foreground);
  font-size: .78rem;
}

.path-examples ul {
  margin: .4rem 0 0;
  padding-left: 1.1rem;
}

.path-examples code {
  color: var(--foreground);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

@media (max-width: 700px) {
  .repository-modal {
    max-height: calc(100dvh - 1rem);
  }

  .details-grid {
    grid-template-columns: 1fr;
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
