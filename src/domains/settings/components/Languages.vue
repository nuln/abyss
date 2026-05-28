<template>
  <select name="selectLanguage" @change="change" :value="locale">
    <option v-for="(language, value) in locales" :key="value" :value="value">
      {{ t(language as string) }}
    </option>
  </select>
</template>

<script setup lang="ts">
import { markRaw } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

defineProps<{
  locale?: string;
}>();

const emit = defineEmits<{
  (e: "update:locale", val: string): void;
}>();

// Vue3 reactivity breaks with this configuration
// so we need to use markRaw as a workaround
// https://github.com/vuejs/core/issues/3024
const locales = markRaw({
  auto: "settings.auto",
  de: "Deutsch",
  en: "English",
  es: "Español",
  fr: "Français",
  it: "Italiano",
  ja: "日本語",
  ko: "한국어",
  "pt-br": "Português (Brasil)",
  ru: "Русский",
  "zh-cn": "中文 (简体)",
  "zh-tw": "中文 (繁體)",
} as Record<string, string>);

const change = (event: Event) => {
  emit("update:locale", (event.target as HTMLSelectElement).value);
};
</script>
