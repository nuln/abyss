import type { RouteRecordRaw } from "vue-router";

export const pluginRoutes: RouteRecordRaw[] = [
    {
        path: "/plugins/template",
        name: "PluginTemplateHome",
        component: () => import("./views/PluginHome.vue"),
    },
];
