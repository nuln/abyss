import * as Vue from "vue";
import * as VueRouter from "vue-router";
import * as Pinia from "pinia";
import * as VueI18n from "vue-i18n";
import * as VueUse from "@vueuse/core";
import * as Lodash from "lodash-es";
import dayjs from "dayjs";
import type { App } from "vue";
import type { Router } from "vue-router";
import type { I18n } from "vue-i18n";
import type { AbyssAPI, AbyssGlobal } from "@/shared/types/abyss";
import type { AbyssPluginManifest, PluginContext, PluginSettingMetadata } from "@/plugin/manifest";

import { emit as emitEvent, on as onEvent } from "@/plugin/events";
import { actionRegistry, registerAction, registerSlot, slotRegistry } from "@/plugin/slots";

import FileCard from "@/plugin/components/FileCard.vue";
import ProgressBar from "@/shared/ui/sdk/ProgressBar.vue";
import Breadcrumbs from "@/domains/files/components/Breadcrumbs.vue";
import Action from "@/shared/ui/header/Action.vue";
import ContextMenu from "@/shared/ui/ContextMenu.vue";
import FileSelectorModal from "@/domains/files/components/FileSelectorModal.vue";
import FileListing from "@/domains/files/views/FileListing.vue";
import Errors from "@/app/Errors.vue";

interface GlobalInitOptions {
    app: App;
    router: Router;
    i18n: I18n;
    Layout: Vue.Component;
    QrcodeVue: Vue.Component;
    registry: AbyssAPI;
}

export function initGlobal(opts: GlobalInitOptions): void {
    const { app, router, i18n, Layout, QrcodeVue, registry } = opts;
    const components: Record<string, Vue.Component> = {};
    const loginButtons = Vue.reactive<any[]>([]);
    const pluginSettings = Vue.reactive<PluginSettingMetadata[]>([]);
    const pluginGlobalSettings = Vue.reactive<PluginSettingMetadata[]>([]);
    const registeredPlugins = new Set<string>();

    window.__ABYSS__ = {
        Vue,
        Router: VueRouter,
        Pinia,
        I18n: VueI18n,
        VueUse,
        Lodash,
        dayjs,
        router,
        i18n,
        Layout,
        QrcodeVue,
        api: registry,
        components,
        actions: actionRegistry,
        slots: slotRegistry,
        loginButtons,
        pluginSettings,
        pluginGlobalSettings,
        events: Vue.reactive({}),
        emit: (event: string, ...args: any[]) => emitEvent(event, ...args),
        on: (event: string, callback: any) => {
            onEvent(event, callback);
        },
        registerComponent: (name: string, component: Vue.Component) => {
            components[name] = component;
            app.component(name, component);
        },
        registerAction: (slot: string, action: any) => {
            registerAction(slot, action);
        },
        registerSlotComponent: (slot: string, component: Vue.Component) => {
            registerSlot(slot, component);
        },
        registerLoginButton: (component: Vue.Component) => {
            loginButtons.push(component);
        },
        registerPlugin: (manifest: AbyssPluginManifest) => {
            const abyss = window.__ABYSS__;
            const pluginSlug = manifest.slugName || manifest.id;

            if (registeredPlugins.has(pluginSlug)) return;
            registeredPlugins.add(pluginSlug);

            if (manifest.settings && Array.isArray(manifest.settings)) {
                manifest.settings.forEach((s: PluginSettingMetadata) => {
                    s.slug = s.slug || pluginSlug;
                    abyss.pluginSettings.push(s);
                });
            }
            if (manifest.globalSettings && Array.isArray(manifest.globalSettings)) {
                manifest.globalSettings.forEach((s: PluginSettingMetadata) => {
                    s.slug = s.slug || pluginSlug;
                    abyss.pluginGlobalSettings.push(s);
                });
            }

            if (manifest.i18n) {
                for (const [locale, messages] of Object.entries(manifest.i18n)) {
                    (abyss.i18n.global as any).mergeLocaleMessage(locale, messages);
                }
            }

            const ctx: PluginContext = {
                router: abyss.router,
                Layout: abyss.Layout,
                api: abyss.api,
                registerComponent: abyss.registerComponent,
                registerAction: abyss.registerAction,
                registerSlotComponent: abyss.registerSlotComponent,
                registerLoginButton: abyss.registerLoginButton,
                emit: abyss.emit,
                on: abyss.on,
            };
            manifest.register(ctx);
        },
    } as AbyssGlobal;

    window.__ABYSS__.registerComponent("FileCard", FileCard);
    window.__ABYSS__.registerComponent("ProgressBar", ProgressBar);
    window.__ABYSS__.registerComponent("Breadcrumbs", Breadcrumbs);
    window.__ABYSS__.registerComponent("Action", Action);
    window.__ABYSS__.registerComponent("action", Action);
    window.__ABYSS__.registerComponent("ContextMenu", ContextMenu);
    window.__ABYSS__.registerComponent("FileSelectorModal", FileSelectorModal);
    window.__ABYSS__.registerComponent("FileListing", FileListing);
    window.__ABYSS__.registerComponent("Errors", Errors);
}
