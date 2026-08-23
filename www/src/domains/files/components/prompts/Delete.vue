<template>
  <div class="card floating">
    <div class="card-content">
      <p v-if="!fileStore.isListing || fileStore.selectedCount === 1">
        {{
          trashEnabled
            ? t("prompts.deleteMessageSingle")
            : t("prompts.deleteMessageSinglePermanent")
        }}
      </p>
      <p v-else>
        {{
          trashEnabled
            ? t("prompts.deleteMessageMultiple", {
                count: fileStore.selectedCount,
              })
            : t("prompts.deleteMessageMultiplePermanent", {
                count: fileStore.selectedCount,
              })
        }}
      </p>
      <label class="permanent-delete-option" v-if="trashEnabled">
        <input type="checkbox" v-model="permanent" />
        <span>{{ t("trash.permanentDeleteOption") }}</span>
      </label>
    </div>
    <div class="card-action">
      <button
        @click="layoutStore.closeHovers()"
        class="button button--flat button--grey"
        :aria-label="t('buttons.cancel')"
        :title="t('buttons.cancel')"
        tabindex="2"
      >
        {{ t("buttons.cancel") }}
      </button>
      <button
        id="focus-prompt"
        @click="submit"
        class="button button--flat button--red"
        :aria-label="t('buttons.delete')"
        :title="t('buttons.delete')"
        tabindex="1"
      >
        {{ t("buttons.delete") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, inject, computed, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { files as api } from "@/domains/files/api";
import buttons from "@/shared/utils/buttons";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";
import { usePluginStore } from "@/domains/settings";

const { t } = useI18n();
const route = useRoute();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const pluginStore = usePluginStore();
const $showError = inject<IToastError>("$showError")!;

const trashEnabled = computed(() => {
  const trash = pluginStore.plugins.find((p) => p.slugName === "trash");
  return trash ? trash.enabled : false;
});

const permanent = ref(!trashEnabled.value);

watch(trashEnabled, (enabled) => {
  permanent.value = !enabled;
});

const submit = async () => {
  buttons.loading("delete");

  try {
    if (!fileStore.isListing) {
      await api.remove(route.path, permanent.value);
      buttons.success("delete");

      layoutStore.currentPrompt?.confirm();
      layoutStore.closeHovers();
      return;
    }

    layoutStore.closeHovers();

    if (fileStore.selectedCount === 0) {
      return;
    }

    const items = fileStore.req?.items || [];
    const promises = [];
    for (const index of fileStore.selected) {
      if (items[index]) {
        promises.push(api.remove(items[index].url, permanent.value));
      }
    }

    await Promise.all(promises);
    buttons.success("delete");

    const nearbyIndex = Math.max(0, Math.min(...fileStore.selected) - 1);
    const nearbyItem = fileStore.req?.items?.[nearbyIndex];

    fileStore.preselect = nearbyItem?.path || null;
    fileStore.reload = true;
  } catch (e: any) {
    buttons.done("delete");
    $showError(e);
    if (fileStore.isListing) fileStore.reload = true;
  }
};
</script>

<style scoped>
.permanent-delete-option {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 1rem;
  font-size: 0.9rem;
  cursor: pointer;
}

.permanent-delete-option input {
  cursor: pointer;
}
</style>
