import * as Vue from "vue";
import VueNumberInput from "@chenfengyuan/vue-number-input";
import VueLazyload from "vue-lazyload";
import { createVfm } from "vue-final-modal";
import Toast from "vue-toastification";
import type { PluginOptions } from "vue-toastification";
import createPinia from "@/app/stores";
import { useAuthStore } from "@/domains/auth";
import { logout } from "@/domains/auth/utils";
import { fetchJSON } from "@/shared/api/utils";
import i18n from "@/shared/i18n";
import { useGlobalToast } from "@/shared/composables/useToast";

export function registerProviders(app: ReturnType<typeof Vue.createApp>, router: any) {
  const pinia = createPinia(router);
  const vfm = createVfm();

  app.component(VueNumberInput.name || "vue-number-input", VueNumberInput);
  app.use(VueLazyload);
  app.use(Toast, {
    transition: "Vue-Toastification__bounce",
    maxToasts: 10,
    newestOnTop: true,
    icon: false,
  } satisfies PluginOptions);

  app.use(vfm);
  app.use(i18n);
  app.use(pinia);
  app.use(router);

  fetchJSON.setTokenGetter(() => useAuthStore().jwt);
  fetchJSON.setUnauthorizedHandler(() => {
    void logout();
  });

  useGlobalToast(app);

  app.mixin({
    mounted() {
      (this as any).$el.__vue__ = this;
    },
  });

  app.directive("focus", {
    mounted: async (el) => el.focus(),
  });

  return { pinia, i18n };
}
