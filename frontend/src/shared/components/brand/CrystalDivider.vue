<script setup lang="ts">
import crystalUrl from '@/assets/icons/crystal-green.svg'
</script>

<template>
  <div class="crystal-divider" aria-hidden="true">
    <span />
    <img :src="crystalUrl" alt="" width="22" height="22" />
    <span />
  </div>
</template>

<style scoped>
.crystal-divider {
  display: flex;
  width: min(100%, 16rem);
  align-items: center;
  gap: 0.65rem;
  color: var(--bronze);
}

.crystal-divider span {
  height: 1px;
  flex: 1;
  background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--bronze) 68%, transparent));
}

.crystal-divider span:last-child {
  background: linear-gradient(90deg, color-mix(in srgb, var(--bronze) 68%, transparent), transparent);
}

.crystal-divider img {
  display: block;
  filter: drop-shadow(0 2px 4px rgba(0, 0, 0, 0.4));
}
</style>
