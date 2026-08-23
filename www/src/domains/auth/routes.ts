import type { RouteRecordRaw } from "vue-router";
import Login from "@/domains/auth/views/Login.vue";
import Setup from "@/domains/auth/views/Setup.vue";

export const authRoutes: RouteRecordRaw[] = [
  { path: "/setup", name: "Setup", component: Setup },
  { path: "/login", name: "Login", component: Login },
];
