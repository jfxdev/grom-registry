<script setup lang="ts">
import type { PrincipalKind } from '@/shared/api/models'
import { Bot, UserRound } from '@lucide/vue'
import { Badge } from '../badge'

defineProps<{ kind: PrincipalKind; iconOnly?: boolean }>()
</script>

<template>
  <Badge
    tone="neutral"
    class="principal-type-badge"
    :aria-label="iconOnly ? (kind === 'service_account' ? 'Service account' : 'User') : undefined"
    :title="iconOnly ? (kind === 'service_account' ? 'Service account' : 'User') : undefined"
  >
    <Bot v-if="kind === 'service_account'" :size="12" aria-hidden="true" />
    <UserRound v-else :size="12" aria-hidden="true" />
    <span v-if="!iconOnly">{{ kind === 'service_account' ? 'Service account' : 'User' }}</span>
  </Badge>
</template>

<style scoped>
.principal-type-badge {
  justify-content: center;
  gap: 0.3rem;
  white-space: nowrap;
}
</style>
