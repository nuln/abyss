<template>
  <div class="plugin-container">
    <template v-if="pluginComponent">
      <component :is="pluginComponent" v-bind="pageProps" />
    </template>
    <div v-else class="plugin-loading">
      <div class="spinner">
        <div class="bounce1"></div>
        <div class="bounce2"></div>
        <div class="bounce3"></div>
      </div>
      <span>{{ $t("files.loading") }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { IPluginPage } from "@/shared/types/plugin";

const props = defineProps<{
  page: IPluginPage;
  pageProps?: Record<string, any>;
}>();

const pluginComponent = computed(() => {
  const abyss = (window as any).__ABYSS__;
  if (!abyss || !abyss.components) return null;
  return abyss.components[props.page.component] || null;
});
</script>

<style scoped>
.plugin-container {
  width: 100%;
  height: 100%;
  min-height: calc(100vh - 64px);
  display: flex;
  flex-direction: column;
}

.plugin-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  gap: 1rem;
  color: var(--text-secondary);
}
</style>
