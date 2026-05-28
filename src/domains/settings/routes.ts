import type { RouteRecordRaw } from "vue-router";
import { FilesLayout } from "@/domains/files";
import Settings from "@/domains/settings/views/Settings.vue";
import GlobalSettings from "@/domains/settings/views/Global.vue";
import ProfileSettings from "@/domains/settings/views/Profile.vue";
import Users from "@/domains/settings/views/Users.vue";
import User from "@/domains/settings/views/User.vue";
import Plugins from "@/domains/settings/views/Plugins.vue";

export const settingsRoutes: RouteRecordRaw[] = [
  {
    path: "/settings",
    component: FilesLayout,
    meta: { requiresAuth: true },
    children: [
      {
        path: "",
        name: "Settings",
        component: Settings,
        redirect: { path: "/settings/profile" },
        children: [
          { path: "profile", name: "ProfileSettings", component: ProfileSettings },
          { path: "global", name: "GlobalSettings", component: GlobalSettings, meta: { requiresAdmin: true } },
          { path: "users", name: "Users", component: Users, meta: { requiresAdmin: true } },
          { path: "users/:id", name: "User", component: User, meta: { requiresAdmin: true } },
          { path: "plugins", name: "Plugins", component: Plugins, meta: { requiresAdmin: true } },
          { path: ":plugin", name: "PluginSettings", component: () => import("@/domains/settings/views/PluginDynamicSettings.vue"), meta: { requiresAdmin: true } },
        ],
      },
    ],
  },
];
