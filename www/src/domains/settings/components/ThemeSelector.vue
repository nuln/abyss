<template>
  <select
    name="selectTheme"
      v-model="localTheme"
  >
    <option value="auto">{{ t('settings.themes.auto') }}</option>
    <option value="light">{{ t('settings.themes.light') }}</option>
    <option value="dark">{{ t('settings.themes.dark') }}</option>
  </select>
</template>

<script setup lang="ts">
  import { computed } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

  const props = defineProps<{
  theme: string;
}>();

const emit = defineEmits<{
  (e: "update:theme", val: string): void;
}>();

  const localTheme = computed({
    get: () => props.theme || "auto",
    set: (val: string) => emit("update:theme", val),
  });
</script>
