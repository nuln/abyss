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
        id="focus-prompt"
        @click="submit"
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
import { useLayoutStore } from "@/app/stores/layout";
import { useI18n } from "vue-i18n";

const { t } = useI18n();
const layoutStore = useLayoutStore();

const props = defineProps<{
  title?: string;
  message?: string;
  confirm?: () => void;
}>();

const displayTitle = computed(() => props.title || "Info");

const submit = () => {
  if (props.confirm) {
    props.confirm();
  }
  layoutStore.closeHovers();
};
</script>
