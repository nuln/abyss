<template>
  <div class="card floating">
    <div class="card-title">
      <h2>{{ t("prompts.rename") }}</h2>
    </div>

    <div class="card-content">
      <p>
        {{ t("prompts.renameMessage") }} <code>{{ oldName }}</code>:
      </p>
      <input
        id="focus-prompt"
        class="input input--block"
        type="text"
        @keyup.enter="submit"
        v-model.trim="name"
      />
    </div>

    <div class="card-action">
      <button
        class="button button--flat button--grey"
        @click="layoutStore.closeHovers()"
        :aria-label="t('buttons.cancel')"
        :title="t('buttons.cancel')"
      >
        {{ t("buttons.cancel") }}
      </button>
      <button
        @click="submit"
        class="button button--flat"
        type="submit"
        :aria-label="t('buttons.rename')"
        :title="t('buttons.rename')"
      >
        {{ t("buttons.rename") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";
import url from "@/shared/utils/url";
import { files as api } from "@/domains/files/api";
import { removePrefix } from "@/domains/files/api";

const { t } = useI18n();
const router = useRouter();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const $showError = inject<IToastError>("$showError")!;

const oldName = computed(() => {
  if (!fileStore.isListing) {
    return fileStore.req?.name || "";
  }

  if (fileStore.selectedCount === 0 || fileStore.selectedCount > 1) {
    return "";
  }

  return fileStore.req?.items?.[fileStore.selected[0]]?.name || "";
});

const name = ref(oldName.value);

const submit = async () => {
  let oldLink = "";
  let newLink = "";

  if (!fileStore.isListing) {
    oldLink = fileStore.req?.url || "";
  } else {
    oldLink = fileStore.req?.items?.[fileStore.selected[0]]?.url || "";
  }
  if (!oldLink) return;

  newLink =
    url.removeLastDir(oldLink) + "/" + encodeURIComponent(name.value);

  try {
    await api.move([{ from: oldLink, to: newLink }]);
    if (!fileStore.isListing) {
      router.push({ path: newLink });
      return;
    }

    fileStore.preselect = removePrefix(newLink);
    fileStore.reload = true;
  } catch (e: any) {
    $showError(e);
  }

  layoutStore.closeHovers();
};
</script>
