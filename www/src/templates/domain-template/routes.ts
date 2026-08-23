import type { RouteRecordRaw } from "vue-router";

export const domainRoutes: RouteRecordRaw[] = [
    {
        path: "/domain",
        name: "DomainHome",
        component: () => import("./views/DomainHome.vue"),
        meta: { requiresAuth: true },
    },
];
