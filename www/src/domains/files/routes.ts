import type { RouteRecordRaw } from "vue-router";
import Layout from "@/domains/files/views/Layout.vue";
import Files from "@/domains/files/views/Files.vue";

export const fileRoutes: RouteRecordRaw[] = [
  {
    path: "/files",
    name: "Layout",
    component: Layout,
    meta: { requiresAuth: true },
    children: [{ path: ":path*", name: "Files", component: Files }],
  },
];
