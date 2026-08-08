<script setup lang="ts">
import { Upload } from '@lucide/vue'
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  registryHost: string
  project: string
  repository?: string
}>(), {
  repository: 'your-repository',
})

const imageReference = computed(() => `${props.registryHost}/${props.project}/${props.repository}:tag`)
const commands = computed(() => `docker push ${imageReference.value}`)
</script>

<template>
  <section class="docker-push-banner" aria-label="Docker push example">
    <div class="docker-push-banner-icon"><Upload :size="18" /></div>
    <div class="min-w-0 flex-1">
      <h2>Push an image</h2>
      <p>Push a tagged local image to this {{ repository === 'your-repository' ? 'project' : 'repository' }}.</p>
      <code>{{ commands }}</code>
    </div>
  </section>
</template>

<style scoped>
.docker-push-banner {
  display: flex;
  align-items: flex-start;
  gap: .8rem;
  border: 1px solid color-mix(in srgb, var(--accent) 16%, var(--border));
  border-radius: .7rem;
  background: color-mix(in srgb, var(--accent) 3%, transparent);
  padding: 1rem;
}

.docker-push-banner-icon {
  display: grid;
  flex: none;
  width: 2.15rem;
  height: 2.15rem;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--accent) 26%, var(--border));
  border-radius: .6rem;
  color: var(--accent);
}

.docker-push-banner h2, .docker-push-banner p { margin: 0; }
.docker-push-banner h2 { font-size: .88rem; font-weight: 700; }
.docker-push-banner p { margin-top: .2rem; color: var(--muted-foreground); font-size: .76rem; line-height: 1.45; }
.docker-push-banner code { display: block; margin-top: .65rem; overflow-x: auto; color: var(--accent); font-size: .75rem; line-height: 1.6; white-space: pre; }
</style>
