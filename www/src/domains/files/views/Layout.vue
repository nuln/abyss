<template>
  <div>
    <div v-if="uploadStore.totalBytes" class="progress">
      <div
        v-bind:style="{
          width: sentPercent + '%',
        }"
      ></div>
    </div>
    <sidebar></sidebar>
    <main :class="{ 'full-width': (isSharePage && !authStore.isLoggedIn) || (!layoutStore.sidebarVisible && authStore.isLoggedIn) }">
      <router-view></router-view>
    </main>

    <upload-files></upload-files>
  </div>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/app/stores/layout";
import { useFileStore } from "@/domains/files/store";
import { useUploadStore } from "@/domains/files/store";
import { useAuthStore } from "@/domains/auth";
import Sidebar from "@/domains/files/components/Sidebar.vue";

import UploadFiles from "@/domains/files/components/prompts/UploadFiles.vue";
import { computed, watch } from "vue";
import { useRoute } from "vue-router";

const layoutStore = useLayoutStore();
const fileStore = useFileStore();
const uploadStore = useUploadStore();
const authStore = useAuthStore();
const route = useRoute();

const isSharePage = computed(() => route.path.startsWith("/share"));

const sentPercent = computed(() =>
  ((uploadStore.sentBytes / uploadStore.totalBytes) * 100).toFixed(2)
);

watch(route, () => {
  fileStore.selected = [];
  fileStore.multiple = false;
  if (layoutStore.currentPromptName !== "success") {
    layoutStore.closeHovers();
  }
});
</script>
