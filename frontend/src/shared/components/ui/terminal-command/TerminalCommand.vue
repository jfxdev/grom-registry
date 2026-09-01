<script setup lang="ts">
import { Button } from '@/shared/components/ui/button'
import { writeClipboardText } from '@/shared/lib/clipboard'
import { Check, Copy } from '@lucide/vue'
import { ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  command: string
  ariaLabel?: string
  copyLabel?: string
  copiedLabel?: string
}>(), {
  ariaLabel: 'Copy command',
  copyLabel: 'Copy',
  copiedLabel: 'Copied',
})

const copied = ref(false)
const copyError = ref('')

watch(() => props.command, () => {
  copied.value = false
  copyError.value = ''
})

async function copyCommand() {
  copyError.value = ''
  const result = await writeClipboardText(props.command)
  if (result === 'copied') {
    copied.value = true
    return
  }
  copied.value = false
  copyError.value = 'Could not copy the command. Select and copy it manually.'
}
</script>

<template>
  <section class="terminal-command" aria-label="Terminal command">
    <header class="terminal-command-header">
      <span class="terminal-command-title"><span class="terminal-command-prompt" aria-hidden="true">$_</span> Terminal</span>
      <Button variant="ghost" size="sm" type="button" :aria-label="ariaLabel" @click="copyCommand">
        <Check v-if="copied" :size="14" />
        <Copy v-else :size="14" />
        {{ copied ? copiedLabel : copyLabel }}
      </Button>
    </header>
    <pre><code><span class="terminal-command-prefix" aria-hidden="true">$ </span>{{ command }}</code></pre>
    <p v-if="copyError" class="terminal-command-error" role="alert">{{ copyError }}</p>
  </section>
</template>

<style scoped>
.terminal-command {
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--accent) 24%, var(--border));
  border-radius: .65rem;
  background: #090d0a;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, .035);
}

.terminal-command-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: .75rem;
  border-bottom: 1px solid color-mix(in srgb, var(--accent) 14%, var(--border));
  background: rgba(145, 173, 36, .055);
  padding: .35rem .45rem .35rem .7rem;
}

.terminal-command-title {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: .45rem;
  color: var(--muted-foreground);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: .7rem;
  font-weight: 700;
  letter-spacing: .06em;
  text-transform: uppercase;
}

.terminal-command-prompt,
.terminal-command-prefix {
  color: var(--accent);
}

.terminal-command pre {
  min-width: 0;
  margin: 0;
  padding: .75rem .85rem;
  color: #c7df62;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: .78rem;
  line-height: 1.6;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  word-break: break-word;
}

.terminal-command code {
  font: inherit;
}

.terminal-command-error {
  margin: 0;
  border-top: 1px solid color-mix(in srgb, var(--destructive) 30%, var(--border));
  padding: .55rem .85rem;
  color: var(--destructive);
  font-size: .76rem;
}
</style>
