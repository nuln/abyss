import type { AbyssPluginManifest } from "@/plugin/manifest";

export const pluginManifest: AbyssPluginManifest = {
    id: "plugin-template",
    slugName: "plugin-template",
    name: "Plugin Template",
    version: "0.1.0",
    sdkVersion: "1.0.0",
    capabilities: ["route", "slot"],
    register() {
        // Implement plugin registration in register.ts.
    },
};
