<template>
  <div class="domain-panel">
    <slot />
  </div>
</template>

<script setup lang="ts"></script>

<style scoped>
.domain-panel {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  background: var(--surface-card);
}
</style>
