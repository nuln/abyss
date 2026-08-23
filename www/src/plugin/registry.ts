import { fetchJSON, fetchURL } from "@/shared/api/utils";
import * as auth from "@/domains/auth/utils";
import { useLayoutStore } from "@/app/stores/layout";
import { useAuthStore } from "@/domains/auth";
import { baseURL, name as siteName } from "@/shared/utils/constants";
import { useFileStore } from "@/domains/files/store";
import * as sdk from "@/plugin";
import { settings } from "@/domains/settings/api";
import dayjs from "dayjs";
import type { App } from "vue";
import type { I18n } from "vue-i18n";
import type { AbyssAPI } from "@/shared/types/abyss";

import FileCard from "@/plugin/components/FileCard.vue";
import ProgressBar from "@/shared/ui/sdk/ProgressBar.vue";
import Breadcrumbs from "@/domains/files/components/Breadcrumbs.vue";
import Action from "@/shared/ui/header/Action.vue";
import ContextMenu from "@/shared/ui/ContextMenu.vue";
import FileSelectorModal from "@/domains/files/components/FileSelectorModal.vue";

/**
 * Creates a fetchJSON wrapper that automatically shows errors via the app's global toast.
 */
function createFetchJSON(app: App) {
    return async <T>(url: string, opts?: any): Promise<T> => {
        try {
            return await fetchJSON<T>(url, opts);
        } catch (err) {
            if (!opts?.silent) {
                (app.config.globalProperties as any).$showError(err);
            }
            throw err;
        }
    };
}

/**
 * Creates the AbyssAPI registry object that is exposed to plugins via `window.__ABYSS__.api`.
 */
export function createRegistry(app: App, router: any, i18n: I18n): AbyssAPI {
    const wrappedFetchJSON = createFetchJSON(app);

    return {
        fetchJSON: wrappedFetchJSON,
        get: <T>(url: string, opts?: any) => wrappedFetchJSON<T>(url, { ...opts, method: "GET" }),
        post: <T>(url: string, opts?: any) => wrappedFetchJSON<T>(url, { ...opts, method: "POST" }),
        put: <T>(url: string, opts?: any) => wrappedFetchJSON<T>(url, { ...opts, method: "PUT" }),
        patch: <T>(url: string, opts?: any) => wrappedFetchJSON<T>(url, { ...opts, method: "PATCH" }),
        delete: <T>(url: string, opts?: any) => wrappedFetchJSON<T>(url, { ...opts, method: "DELETE" }),
        auth: {
            login: (email: string, password: string, captcha?: string) =>
                auth.login(email, password, captcha || ""),
            signup: auth.signup,
            parseToken: auth.parseToken,
            storeRefreshToken: auth.storeRefreshToken,
            logout: auth.logout,
        },
        ui: {
            showError: (error: any) =>
                (app.config.globalProperties as any).$showError(error),
            showSuccess: (message: string) =>
                (app.config.globalProperties as any).$showSuccess(message),
            showHover: (options: any) => {
                const layoutStore = useLayoutStore();
                layoutStore.showHover(options);
            },
            closeHovers: () => {
                const layoutStore = useLayoutStore();
                layoutStore.closeHovers();
            },
        },
        stores: {
            file: useFileStore,
            layout: useLayoutStore,
            auth: useAuthStore,
        },
        constants: {
            baseURL,
            name: siteName,
        },
        fetchURL,
        utils: {
            baseURL,
        },
        router: {
            push: router.push,
            replace: router.replace,
        },
        t: (key: string, values?: any) => (i18n.global as any).t(key, values),
        get user() {
            return useAuthStore().user;
        },
        set user(val) {
            useAuthStore().setUser(val);
        },
        settings,
        sdk: {
            ...sdk,
            fetchJSON: wrappedFetchJSON,
            components: {
                FileCard,
                ProgressBar,
                Breadcrumbs,
                Action,
                ContextMenu,
                FileSelectorModal,
            },
        },
        dayjs,
    } as AbyssAPI;
}
