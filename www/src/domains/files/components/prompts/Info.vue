<template>
  <div class="card floating">
    <div class="card-title">
      <h2>{{ t("prompts.fileInfo") }}</h2>
    </div>

    <div class="card-content">
      <p v-if="fileStore.selected.length > 1">
        {{ t("prompts.filesSelected", { count: fileStore.selected.length }) }}
      </p>

      <p class="break-word" v-if="fileStore.selected.length < 2">
        <strong>{{ t("prompts.displayName") }}</strong> {{ name }}
      </p>

      <p v-if="!dir || fileStore.selected.length > 1">
        <strong>{{ t("prompts.size") }}:</strong>
        <span id="content_length"></span> {{ humanSize }}
      </p>

      <div v-if="resolution">
        <strong>{{ t("prompts.resolution") }}:</strong>
        {{ resolution.width }} x {{ resolution.height }}
      </div>

      <p v-if="fileStore.selected.length < 2" :title="modTime">
        <strong>{{ t("prompts.lastModified") }}:</strong> {{ humanTime }}
      </p>

      <template v-if="dir && fileStore.selected.length === 0">
        <p>
          <strong>{{ t("prompts.numberFiles") }}:</strong> {{ fileStore.req?.numFiles }}
        </p>
        <p>
          <strong>{{ t("prompts.numberDirs") }}:</strong> {{ fileStore.req?.numDirs }}
        </p>
      </template>
    </div>

    <div class="card-action">
      <button
        id="focus-prompt"
        type="submit"
        @click="layoutStore.closeHovers()"
        class="button button--flat"
        :aria-label="t('buttons.ok')"
        :title="t('buttons.ok')"
      >
        {{ t("buttons.ok") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";
import { filesize } from "@/shared/utils";
import dayjs from "dayjs";

const { t } = useI18n();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();

const humanSize = computed(() => {
  if (fileStore.selectedCount === 0 || !fileStore.isListing) {
    return filesize(fileStore.req?.size || 0);
  }

  let sum = 0;
  for (const selected of fileStore.selected) {
    const item = fileStore.req?.items?.[selected];
    if (item) sum += item.size;
  }
  return filesize(sum);
});

const humanTime = computed(() => {
  if (
    fileStore.selectedCount === 0 ||
    !fileStore.req?.items?.[fileStore.selected[0]]
  ) {
    return dayjs(fileStore.req?.modified).fromNow();
  }
  const selectedItem = fileStore.req?.items?.[fileStore.selected[0]];
  if (!selectedItem) return "";
  return dayjs(selectedItem.modified).fromNow();
});

const modTime = computed(() => {
  const item = fileStore.req?.items?.[fileStore.selected[0]];
  if (fileStore.selectedCount === 0 || !item) {
    return new Date(Date.parse(fileStore.req?.modified || "")).toLocaleString();
  }
  return new Date(Date.parse(item.modified)).toLocaleString();
});

const name = computed(() =>
  fileStore.selectedCount === 0
    ? fileStore.req?.name
    : fileStore.req?.items?.[fileStore.selected[0]]?.name
);

const dir = computed(() =>
  fileStore.selectedCount > 1 ||
  (fileStore.selectedCount === 0
    ? fileStore.req?.isDir
    : fileStore.req?.items?.[fileStore.selected[0]]?.isDir)
);

const resolution = computed(() => {
  if (fileStore.selectedCount === 1) {
    const selectedItem = fileStore.req?.items?.[fileStore.selected[0]] as any;
    if (selectedItem && selectedItem.type === "image") {
      return selectedItem.resolution;
    }
  } else if (fileStore.req && (fileStore.req as any).type === "image") {
    return (fileStore.req as any).resolution;
  }
  return null;
});
</script>
