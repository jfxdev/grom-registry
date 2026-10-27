<script setup lang="ts">
import type { RepositoryProfile } from '@/shared/api/models'
import { TerminalCommand } from '@/shared/components/ui/terminal-command'
import { artifactRecipe } from '@/shared/constants'
import { Upload } from '@lucide/vue'
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  registryHost: string
  project: string
  repository?: string
  profile?: RepositoryProfile
}>(), {
  repository: 'your-repository',
  profile: 'unknown',
})

const recipe = computed(() => artifactRecipe(props.profile))
const reference = computed(() => ({
  host: props.registryHost,
  path: `${props.project}/${props.repository}`,
  tag: 'tag',
}))
const commands = computed(() => recipe.value.pushCommand(reference.value))
const description = computed(() =>
  props.repository === 'your-repository'
    ? recipe.value.description.replace('this repository', 'a repository in this project')
    : recipe.value.description,
)
</script>

<template>
  <section class="artifact-push-banner" aria-label="Push example">
    <div class="artifact-push-banner-icon"><Upload :size="18" /></div>
    <div class="min-w-0 flex-1">
      <h2>{{ recipe.title }}</h2>
      <p>{{ description }}</p>
      <TerminalCommand :command="commands" aria-label="Copy push command" />
      <p v-if="recipe.note" class="artifact-push-banner-note">{{ recipe.note }}</p>
    </div>
  </section>
</template>

<style scoped>
.artifact-push-banner {
  display: flex;
  align-items: flex-start;
  gap: .8rem;
  border: 1px solid color-mix(in srgb, var(--accent) 16%, var(--border));
  border-radius: .7rem;
  background: color-mix(in srgb, var(--accent) 3%, transparent);
  padding: 1rem;
}

.artifact-push-banner-icon {
  display: grid;
  flex: none;
  width: 2.15rem;
  height: 2.15rem;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--accent) 26%, var(--border));
  border-radius: .6rem;
  color: var(--accent);
}

.artifact-push-banner h2, .artifact-push-banner p { margin: 0; }
.artifact-push-banner h2 { font-size: .88rem; font-weight: 700; }
.artifact-push-banner p { margin-top: .2rem; color: var(--muted-foreground); font-size: .76rem; line-height: 1.45; }
.artifact-push-banner-note { margin-top: .55rem; }
.terminal-command { margin-top: .65rem; }
</style>
