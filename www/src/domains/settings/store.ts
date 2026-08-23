import { defineStore } from "pinia";
import { fetchJSON } from "@/domains/settings/api";
import type { IPluginPage } from "@/shared/types/plugin";
import i18n from "@/shared/i18n";

export interface IPluginInfo {
    slugName: string;
    name: string;
    description: string;
    version: string;
    type: "free" | "paid";
    enabled: boolean;
    status: "healthy" | "unhealthy" | "unknown";
    healthMessage?: string;
    hasUI: boolean;
    hasConfig: boolean;
    requireConfig: boolean;
}

export interface IConfigField {
    name: string;
    type: string;
    title: string;
    description?: string;
    required: boolean;
    value: any;
    options?: { label: string; value: string }[];
    readOnly?: boolean;
    copyable?: boolean;
    action?: string;
    icon?: string;
    iconClass?: string;
    row?: number;
    group?: string;
}

export const usePluginStore = defineStore("plugins", {
    state: () => ({
        pages: [] as IPluginPage[],
        plugins: [] as IPluginInfo[],
        loaded: false,
    }),
    getters: {
        sidebarPages: (state) =>
            state.pages
                .filter((p: IPluginPage) => p.navPosition === "sidebar")
                .sort((a: IPluginPage, b: IPluginPage) => a.navOrder - b.navOrder),
        settingsTabs: (state) =>
            state.pages
                .filter((p: IPluginPage) => p.navPosition === "settings")
                .sort((a: IPluginPage, b: IPluginPage) => a.navOrder - b.navOrder),
        footerPages: (state) =>
            state.pages
                .filter((p: IPluginPage) => p.navPosition === "sidebar-footer")
                .sort((a: IPluginPage, b: IPluginPage) => a.navOrder - b.navOrder),
    },
    actions: {
        async fetchPluginPages() {
            try {
                const pages = await fetchJSON<IPluginPage[]>("/api/plugins/ui");
                this.pages = pages || [];
                this.loaded = true;
            } catch {
                this.pages = [];
                this.loaded = true;
            }
        },
        async fetchPlugins() {
            try {
                this.plugins = await fetchJSON<IPluginInfo[]>("/api/plugins/list");
            } catch {
                this.plugins = [];
            }
        },
        async togglePlugin(slug: string, enabled: boolean) {
            await fetchJSON(`/api/settings/plugins/${slug}/enable`, {
                method: "POST",
                body: JSON.stringify({ enabled }),
            });
            const p = this.plugins.find((item) => item.slugName === slug);
            if (p) p.enabled = enabled;

            await Promise.all([this.fetchPluginPages(), this.fetchPlugins()]);
        },
        async fetchPluginConfig(slug: string): Promise<IConfigField[]> {
            return fetchJSON<IConfigField[]>(`/api/settings/plugins/${slug}/config`);
        },
        async updatePluginConfig(slug: string, config: any) {
            await fetchJSON(`/api/settings/plugins/${slug}/config`, {
                method: "PUT",
                body: JSON.stringify(config),
            });
        },
        async fetchPluginI18n() {
            try {
                const messages = (await fetchJSON("/api/plugins/i18n")) as Record<string, any>;
                if (!messages) return;
                
                // Debugging: Attach to window so we can inspect in console
                (window as any).pluginMessages = messages;
                
                Object.keys(messages).forEach(locale => {
                    // Try to match the locale case-insensitively if needed
                    const targetLocale = locale.toLowerCase();
                    const current = i18n.global.getLocaleMessage(targetLocale) || {};
                    const newMessages = messages[locale];
                    
                    // Simple merge for the top-level keys (plugins)
                    for (const key in newMessages) {
                        i18n.global.mergeLocaleMessage(targetLocale, { [key]: newMessages[key] });
                    }
                    
                    console.log(`Merged ${Object.keys(newMessages).length} plugins for locale ${targetLocale}`);
                });
            } catch (e) {
                console.error("Failed to fetch plugin i18n", e);
            }
        },
    },
});
