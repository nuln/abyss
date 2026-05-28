import type { Component } from "vue";

export interface PluginSlotBinding {
    name: string;
    component: Component;
}

export const pluginSlots: PluginSlotBinding[] = [
    {
        name: "dashboard-widgets",
        component: () => import("./components/PluginCard.vue"),
    },
];
