<template>
  <div :class="{ 'centered-error-container': error }">
    <breadcrumbs v-if="error || fileStore.req?.type === undefined" base="/files" />
    <errors v-if="error" :errorCode="error.status" inline />
    <component v-else-if="currentView" :is="currentView"></component>
    <div v-else>
      <h2 class="message delayed">
        <div class="spinner">
          <div class="bounce1"></div>
          <div class="bounce2"></div>
          <div class="bounce3"></div>
        </div>
        <span>{{ t("files.loading") }}</span>
      </h2>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  onBeforeUnmount,
  onMounted,
  onUnmounted,
  ref,
  watch,
} from "vue";
import { files as api } from "@/domains/files/api";
import { storeToRefs } from "pinia";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";

import Breadcrumbs from "@/domains/files/components/Breadcrumbs.vue";
import Errors from "@/app/Errors.vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";
import FileListing from "@/domains/files/views/FileListing.vue";
import { StatusError } from "@/domains/files/api";
import { name } from "@/shared/utils/constants";

const Editor = defineAsyncComponent(() => import("@/domains/files/views/Editor.vue"));
const Preview = defineAsyncComponent(() => import("@/domains/files/views/Preview.vue"));

const layoutStore = useLayoutStore();
const fileStore = useFileStore();

const { reload } = storeToRefs(fileStore);

const route = useRoute();

const { t } = useI18n({});

let fetchDataController = new AbortController();

const error = ref<StatusError | null>(null);

const currentView = computed(() => {
  if (fileStore.req?.type === undefined) {
    return null;
  }

  if (fileStore.req.isDir) {
    return FileListing;
  } else if (fileStore.req.extension.toLowerCase() === ".csv") {
    // CSV files use Preview for table view, unless ?edit=true
    if (route.query.edit === "true") {
      return Editor;
    }
    return Preview;
  } else if (
    fileStore.req.type === "text" ||
    fileStore.req.type === "textImmutable"
  ) {
    return Editor;
  } else {
    return Preview;
  }
});

// Define hooks
onMounted(() => {
  fetchData();
  fileStore.isFiles = true;
  window.addEventListener("keydown", keyEvent);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", keyEvent);
});

onUnmounted(() => {
  fileStore.isFiles = false;
  fileStore.updateRequest(null);
  fetchDataController.abort();
});

watch(route, () => {
  fetchData();
});
watch(reload, (newValue) => {
  newValue && fetchData();
});

// Define functions

const applyPreSelection = () => {
  const preselect = fileStore.preselect;
  fileStore.preselect = null;

  if (!fileStore.req?.isDir || fileStore.oldReq === null) return;

  let index = -1;
  const items = fileStore.req?.items || [];
  if (preselect) {
    // Find item with the specified path
    index = items.findIndex((item) => item.path === preselect);
  } else if (fileStore.oldReq?.path?.startsWith(fileStore.req.path)) {
    // Get immediate child folder of the previous path
    const name = fileStore.oldReq.path
      .substring(fileStore.req.path.length)
      .split("/")
      .shift();

    if (fileStore.req?.path && name) {
      index = items.findIndex((val) => val.path == fileStore.req!.path + name);
    }
  }

  if (index === -1) return;
  fileStore.selected.push(index);
};

const fetchData = async () => {
  // Reset view information.
  fileStore.reload = false;
  fileStore.selected = [];
  fileStore.multiple = false;
  layoutStore.closeHovers();

  // Set loading to true and reset the error.
  layoutStore.loading = true;
  error.value = null;

  let url = route.path;
  if (url === "") url = "/";
  if (url[0] !== "/") url = "/" + url;
  // Cancel the ongoing request
  fetchDataController.abort();
  fetchDataController = new AbortController();
  try {
    const res = await api.fetch(url, fetchDataController.signal);
    fileStore.updateRequest(res);
    document.title = `${res.name || t("sidebar.myFiles")} - ${t("files.files")} - ${name}`;
    layoutStore.loading = false;

    // Selects the post-reload target item or the previously visited child folder
    applyPreSelection();
  } catch (err) {
    if (err instanceof Error && "is_canceled" in err && (err as any).is_canceled) {
      return;
    }
    if (err instanceof Error) {
      error.value = err;
    }
    layoutStore.loading = false;
  }
};
const keyEvent = (event: KeyboardEvent) => {
  if (event.key === "F1") {
    event.preventDefault();
    layoutStore.showHover("help");
  }
};
</script>

<style scoped>
.centered-error-container {
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - 120px); /* Give it some space to center vertically */
}

/* Ensure breadcrumbs and errors both take full width inside the flex container */
.centered-error-container > * {
  width: 100%;
}
</style>
