import { ref, onMounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { setHtmlLocale } from "@/shared/i18n";
import { getMediaPreference, getTheme, setTheme } from "@/shared/theme";
import { useLayoutStore } from "@/app/stores/layout";
import { useTaskStore } from "@/domains/tasks/store";
import { useAuthStore } from "@/domains/auth";
import { usePluginStore } from "@/domains/settings";

import { loadPlugin } from "@/plugin/loader";

export function useAppBoot() {
  const { locale } = useI18n();
  const layoutStore = useLayoutStore();
  const taskStore = useTaskStore();
  const authStore = useAuthStore();
  const pluginStore = usePluginStore();

  const userTheme = ref<UserTheme>(getTheme() || getMediaPreference());

  onMounted(() => {
    setTheme(userTheme.value);
    setHtmlLocale(locale.value);
    const loading = document.getElementById("loading");
    loading?.classList.add("done");
    setTimeout(() => {
      loading?.parentNode?.removeChild(loading);
    }, 200);
  });

  watch(
    () => authStore.isLoggedIn,
    async (isLoggedIn) => {
      // Always fetch plugins (public endpoints) to support login buttons and public features
      // Always ensure translations are loaded
      await pluginStore.fetchPluginI18n();

      if (!pluginStore.loaded) {
        await Promise.all([
          pluginStore.fetchPluginPages(),
          pluginStore.fetchPlugins(),
        ]);
      }

      if (isLoggedIn) {
        taskStore.init();
      } else {
        taskStore.stop();
      }
    },
    { immediate: true }
  );

  // Watch for plugin status changes to load UI on-the-fly
  watch(
    () => pluginStore.plugins,
    (plugins) => {
      plugins.forEach((p) => {
        if (p.enabled && p.hasUI) {
          loadPlugin(p.slugName).catch(() => {});
        }
      });
    },
    { deep: true, immediate: true }
  );

  watch(locale, (newValue) => {
    if (newValue) setHtmlLocale(newValue);
  });

  watch(
    () => layoutStore.sidebarVisible,
    (visible) => {
      document.body.classList.toggle("sidebar-hidden", !visible);
    },
    { immediate: true }
  );
}
