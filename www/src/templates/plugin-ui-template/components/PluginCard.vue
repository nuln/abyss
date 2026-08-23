<template>
  <article class="plugin-card">
    <h3>Plugin Card</h3>
    <slot />
  </article>
</template>

<script setup lang="ts"></script>

<style scoped>
.plugin-card {
  padding: var(--space-4);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--surface-card);
}
</style>
