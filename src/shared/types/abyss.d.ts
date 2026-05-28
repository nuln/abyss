import * as Vue from "vue";
import * as VueRouter from "vue-router";
import * as Pinia from "pinia";
import * as VueI18n from "vue-i18n";
import * as VueUse from "@vueuse/core";
import * as Lodash from "lodash-es";
import dayjs from "dayjs";
import type { Router } from "vue-router";
import type { I18n } from "vue-i18n";
import type {
    AbyssPluginManifest,
    PluginContext as RuntimePluginContext,
    PluginSettingMetadata as RuntimePluginSettingMetadata,
} from "@/plugin/manifest";

export interface User {
    id: number;
    username?: string;
    email: string;
    perm: {
        admin: boolean;
        [key: string]: any;
    };
    [key: string]: any;
}

export interface HoverOptions {
    prompt: string;
    props?: Record<string, any>;
    confirm?: (value?: any) => void;
    action?: (value?: any) => void;
}

export interface AbyssSDK {
    formatDate: (val: number | string | Date) => string;
    getThumbnailUrl: (id?: number, size?: string) => string;
    getOriginalUrl: (id?: number) => string;
    getTrashThumbnailUrl: (id: string) => string;
    filesize: (bytes: number) => string;
    dayjs: typeof dayjs;
    auth: any;
    fileApi: any;
    baseURL: string;
    ui: {
        useToast: () => {
            success: (msg: string) => void;
            error: (err: any) => void;
        };
    };
    components: {
        FileCard: Vue.Component;
        ProgressBar: Vue.Component;
    };
}

export interface AbyssAPI {
    fetchJSON: <T>(url: string, options?: RequestInit) => Promise<T>;
    get: <T>(url: string, options?: RequestInit) => Promise<T>;
    post: <T>(url: string, options?: RequestInit) => Promise<T>;
    put: <T>(url: string, options?: RequestInit) => Promise<T>;
    patch: <T>(url: string, options?: RequestInit) => Promise<T>;
    delete: <T>(url: string, options?: RequestInit) => Promise<T>;
    auth: {
        login: (email: string, password: string, captcha?: string) => Promise<{ token: string }>;
        signup: (email: string, username: string, password: string) => Promise<void>;
        parseToken: (token: string) => void;
        storeRefreshToken: (token: string) => void;
        logout: () => void;
    };
    ui: {
        showError: (error: any) => void;
        showSuccess: (message: string) => void;
        showHover: (options: HoverOptions) => void;
        closeHovers: () => void;
    };
    utils: {
        baseURL: string;
    };
    router: {
        push: (to: VueRouter.RouteLocationRaw) => Promise<VueRouter.NavigationFailure | void | undefined>;
        replace: (to: VueRouter.RouteLocationRaw) => Promise<VueRouter.NavigationFailure | void | undefined>;
    };
    t: (key: string, values?: any) => string;
    user: User | null;
    settings: any;
    sdk: AbyssSDK;
}

export interface PluginContext extends RuntimePluginContext {
    router: Router;
    Layout: Vue.Component;
    api: AbyssAPI;
    registerComponent: (name: string, component: Vue.Component) => void;
    registerAction: (slot: string, action: any) => void;
    registerSlotComponent: (slot: string, component: Vue.Component) => void;
    registerLoginButton: (component: Vue.Component) => void;
    emit: (event: string, ...args: any[]) => void;
    on: (event: string, callback: any) => void;
}

export interface PluginManifest extends AbyssPluginManifest {
    register: (ctx: PluginContext) => void;
}

export interface PluginSettingMetadata extends RuntimePluginSettingMetadata {
    component: string;
    label: string;
    icon?: string;
    slug?: string;
    category?: string;
}

export interface AbyssGlobal {
    Vue: typeof Vue;
    Router: typeof VueRouter;
    Pinia: typeof Pinia;
    I18n: typeof VueI18n;
    VueUse: typeof VueUse;
    Lodash: typeof Lodash;
    dayjs: typeof dayjs;
    router: Router;
    i18n: I18n;
    Layout: Vue.Component;
    QrcodeVue: Vue.Component;
    api: AbyssAPI;
    components: Record<string, Vue.Component>;
    actions: Record<string, any[]>;
    slots: Record<string, Vue.Component[]>;
    loginButtons: Vue.Component[];
    pluginSettings: PluginSettingMetadata[];
    pluginGlobalSettings: PluginSettingMetadata[];
    events: Record<string, Function[]>;
    emit: (event: string, ...args: any[]) => void;
    on: (event: string, callback: Function) => void;
    registerComponent: (name: string, component: Vue.Component) => void;
    registerAction: (slot: string, action: any) => void;
    registerSlotComponent: (slot: string, component: Vue.Component) => void;
    registerLoginButton: (component: Vue.Component) => void;
    registerPlugin: (manifest: PluginManifest) => void;
}

declare global {
    interface Window {
        __ABYSS__: AbyssGlobal;
    }
}
