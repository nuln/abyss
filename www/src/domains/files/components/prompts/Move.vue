<template>
  <div class="card floating">
    <div class="card-title">
      <h2>{{ t("prompts.move") }}</h2>
    </div>

    <div class="card-content">
      <p>{{ t("prompts.moveMessage") }}</p>
      <file-list
        ref="fileListRef"
        @update:selected="(val: string) => (dest = val)"
        :exclude="excludedFolders"
        tabindex="1"
      />
    </div>

    <div
      class="card-action"
      style="display: flex; align-items: center; justify-content: space-between"
    >
      <template v-if="authStore.user?.perm?.create">
        <button
          class="button button--flat"
          @click="fileListRef?.createDir()"
          :aria-label="t('sidebar.newFolder')"
          :title="t('sidebar.newFolder')"
          style="justify-self: left"
        >
          <span>{{ t("sidebar.newFolder") }}</span>
        </button>
      </template>
      <div>
        <button
          class="button button--flat button--grey"
          @click="layoutStore.closeHovers()"
          :aria-label="t('buttons.cancel')"
          :title="t('buttons.cancel')"
          tabindex="3"
        >
          {{ t("buttons.cancel") }}
        </button>
        <button
          id="focus-prompt"
          class="button button--flat"
          @click="move"
          :disabled="route.path === dest"
          :aria-label="t('buttons.move')"
          :title="t('buttons.move')"
          tabindex="2"
        >
          {{ t("buttons.move") }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";
import { useAuthStore } from "@/domains/auth";
import FileList from "./FileList.vue";
import { files as api } from "@/domains/files/api";
import buttons from "@/shared/utils/buttons";
import * as upload from "@/domains/files/utils";
import { removePrefix } from "@/domains/files/api";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const authStore = useAuthStore();
const $showError = inject<IToastError>("$showError")!;

const dest = ref<string | null>(null);
const fileListRef = ref<InstanceType<typeof FileList> | null>(null);

const excludedFolders = computed(() => {
  const items = fileStore.req?.items || [];
  return fileStore.selected
    .filter((idx) => items[idx]?.isDir)
    .map((idx) => items[idx]?.url);
});

const move = async (event: Event) => {
  event.preventDefault();
  const items: { from: string; to: string; name: string }[] = [];

  const itemsList = fileStore.req?.items || [];
  for (const item of fileStore.selected) {
    const selectedItem = itemsList[item];
    if (selectedItem) {
      items.push({
        from: selectedItem.url,
        to: (dest.value || "") + encodeURIComponent(selectedItem.name),
        name: selectedItem.name,
      });
    }
  }

  const action = async (overwrite: boolean, rename: boolean) => {
    buttons.loading("move");

    await api
      .move(items, overwrite, rename)
      .then(() => {
        buttons.success("move");
        fileStore.preselect = removePrefix(items[0].to);
        router.push({ path: dest.value! });
      })
      .catch((e: any) => {
        buttons.done("move");
        $showError(e);
      });
  };

  const dstItems = (await api.fetch(dest.value!)).items || [];
  const conflict = upload.checkConflict(items as any, dstItems);

  let overwrite = false;
  let rename = false;

  if (conflict) {
    layoutStore.showHover({
      prompt: "replace-rename",
      confirm: (event: Event, option: string) => {
        overwrite = option == "overwrite";
        rename = option == "rename";

        event.preventDefault();
        layoutStore.closeHovers();
        action(overwrite, rename);
      },
    });
    return;
  }

  action(overwrite, rename);
};
</script>
