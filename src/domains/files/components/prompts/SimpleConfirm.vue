<template>
  <div class="card floating">
    <div class="card-title">
      <h2>{{ displayTitle }}</h2>
    </div>

    <div class="card-content">
      <p>{{ message }}</p>
    </div>

    <div class="card-action">
      <button
        class="button button--flat button--grey"
        @click="cancel"
        :aria-label="t('buttons.cancel')"
        :title="t('buttons.cancel')"
      >
        {{ t("buttons.cancel") }}
      </button>
      <button
        id="focus-prompt"
        @click="submit"
        class="button button--flat button--red"
        :aria-label="t('buttons.ok')"
        :title="t('buttons.ok')"
      >
        {{ t("buttons.ok") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from "vue";
import { useLayoutStore } from "@/app/stores/layout";
import { useI18n } from "vue-i18n";

const { t } = useI18n();
const layoutStore = useLayoutStore();

const props = defineProps<{
  title?: string;
  message?: string;
  confirm?: (result: boolean) => void;
}>();

const submitted = ref(false);

const displayTitle = computed(() => props.title || t("prompts.confirmation"));

onBeforeUnmount(() => {
  if (!submitted.value && props.confirm) {
    props.confirm(false);
  }
});

const submit = () => {
  submitted.value = true;
  if (props.confirm) {
    props.confirm(true);
  }
  layoutStore.closeHovers();
};

const cancel = () => {
  submitted.value = true;
  if (props.confirm) {
    props.confirm(false);
  }
  layoutStore.closeHovers();
};
</script>
