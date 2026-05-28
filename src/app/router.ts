import { createRouter, createWebHistory } from "vue-router";
import Errors from "@/app/Errors.vue";
import { authRoutes } from "@/domains/auth/routes";
import { fileRoutes } from "@/domains/files/routes";
import { settingsRoutes } from "@/domains/settings/routes";
import { taskRoutes } from "@/domains/tasks/routes";
import { useAuthStore } from "@/domains/auth";
import { usePluginStore } from "@/domains/settings";
import { baseURL, loginPage, name, recaptcha } from "@/shared/utils/constants";
import i18n from "@/shared/i18n";
import { login, validateLogin } from "@/domains/auth/utils";
import type { IPluginPage } from "@/shared/types/plugin";
import { loadPlugin } from "@/plugin/loader";

const titles = {
    Login: "sidebar.login",
    Files: "files.files",
    Settings: "sidebar.settings",
    ProfileSettings: "settings.profileSettings",
    Forbidden: "errors.forbidden",
    NotFound: "errors.notFound",
    InternalServerError: "errors.internal",
};

const errorRoutes = [
    {
        path: "/403",
        name: "Forbidden",
        component: Errors,
        props: {
            errorCode: 403,
            showHeader: true,
        },
    },
    {
        path: "/404",
        name: "NotFound",
        component: Errors,
        props: {
            errorCode: 404,
            showHeader: true,
        },
    },
    {
        path: "/500",
        name: "InternalServerError",
        component: Errors,
        props: {
            errorCode: 500,
            showHeader: true,
        },
    },
    {
        path: "/:catchAll(.*)*",
        name: "NotFoundFallback",
        component: Errors,
        props: {
            errorCode: 404,
            showHeader: true,
        },
    },
];

const routes = [
    {
        path: "/",
        redirect: "/files/",
    },
    ...authRoutes,
    ...fileRoutes,
    ...settingsRoutes,
    ...taskRoutes,
    ...errorRoutes,
];

async function initAuth() {
    const authStore = useAuthStore();
    await authStore.checkSetup();

    if (loginPage) {
        await validateLogin();
    } else {
        await login("", "", "");
    }

    if (recaptcha) {
        await new Promise<void>((resolve) => {
            const check = () => {
                if (typeof window.grecaptcha === "undefined") {
                    setTimeout(check, 100);
                } else {
                    resolve();
                }
            };

            check();
        });
    }
}

const router = createRouter({
    history: createWebHistory(baseURL),
    routes,
});

router.beforeResolve(async (to, from, next) => {
    const authStore = useAuthStore();
    const pluginStore = usePluginStore();

    const titleKey = titles[to.name as keyof typeof titles];
    let title = titleKey ? i18n.global.t(titleKey) : (to.name as string);

    if (!titleKey || title === to.name) {
        const pluginPage = pluginStore.pages.find((p) => {
            const pRoute = p.route.replace(/\/$/, "");
            const toPath = to.path.replace(/\/$/, "");
            return toPath === pRoute || toPath.startsWith(pRoute + "/");
        });
        if (pluginPage) {
            title = i18n.global.t(pluginPage.name);
        }
    }

    document.title = title + " - " + name;

    if (from.name == null) {
        try {
            await initAuth();
        } catch {
            // Ignore init auth errors and let guards redirect.
        }
    }

    if (authStore.isLoggedIn && !pluginStore.loaded) {
        await Promise.all([pluginStore.fetchPluginPages(), pluginStore.fetchPlugins()]);
    }

    if (authStore.isLoggedIn) {
        const healthyPlugins = pluginStore.plugins.filter(
            (p) => p.enabled && p.status === "healthy" && p.hasUI,
        );

        if (healthyPlugins.length > 0) {
            const loadPromises = healthyPlugins.map(async (plugin) => {
                try {
                    await loadPlugin(plugin.slugName);
                } catch {
                    // Ignore single plugin load failures.
                }
            });
            await Promise.all(loadPromises);
        }

        const targetPath = to.path === "/404" && to.query.path ? (to.query.path as string) : to.path;
        const normalizedTargetPath = targetPath.split("?")[0].replace(/\/$/, "") || "/";
        const isPluginPage = pluginStore.pages.some((p: IPluginPage) => {
            const pRoute = p.route.replace(/\/$/, "") || "/";
            return normalizedTargetPath === pRoute || normalizedTargetPath.startsWith(pRoute + "/");
        });

        if (isPluginPage) {
            const resolved = router.resolve(targetPath);
            const isResolvedToNotFound =
                resolved.name === "NotFound" ||
                resolved.name === "NotFoundFallback" ||
                resolved.matched.length === 0 ||
                (resolved.matched.length === 1 && resolved.matched[0].name === "NotFound");

            if (to.name === "NotFound" || isResolvedToNotFound) {
                const pluginPage = pluginStore.pages.find((p: IPluginPage) => {
                    const pRoute = p.route.replace(/\/$/, "") || "/";
                    return normalizedTargetPath === pRoute || normalizedTargetPath.startsWith(pRoute + "/");
                });

                if (pluginPage) {
                    // Try to load the plugin if not already loaded
                    await loadPlugin(pluginPage.slugName);

                    // Check if the plugin has registered its own route for this path
                    const reResolved = router.resolve(targetPath);
                    const isNowResolved = reResolved.name !== "NotFound" &&
                        reResolved.name !== "NotFoundFallback" &&
                        reResolved.matched.length > 0;

                    if (isNowResolved) {
                        next({
                            path: targetPath,
                            query: to.query,
                            replace: true,
                        });
                        return;
                    }

                    // Fallback: Register the route dynamically if it still doesn't exist
                    const routeName = `Plugin_${pluginPage.slugName}`;
                    if (!router.hasRoute(routeName)) {
                        router.addRoute("Layout", {
                            path: pluginPage.route,
                            name: routeName,
                            component: () => import("@/plugin/components/PluginContainer.vue"),
                            props: { page: pluginPage },
                            meta: { requiresAuth: true }
                        });
                    }

                    next({
                        path: targetPath,
                        query: to.query,
                        replace: true,
                    });
                    return;
                }
            }
        }
    }

    if (authStore.needsSetup && to.name !== "Setup") {
        next({ name: "Setup" });
        return;
    }

    if (to.name === "Setup" && !authStore.needsSetup) {
        next({ path: "/" });
        return;
    }

    if (to.path.endsWith("/login") && authStore.isLoggedIn) {
        next({ path: "/files/" });
        return;
    }

    if (to.matched.some((record) => record.meta.requiresAuth)) {
        if (!authStore.isLoggedIn) {
            next({
                path: "/login",
                query: { redirect: to.fullPath },
            });
            return;
        }

        if (to.matched.some((record) => record.meta.requiresAdmin)) {
            if (!authStore.user || !authStore.user.perm?.admin) {
                next({ path: "/403" });
                return
            }
        }
    }

    next();
});

export { router, router as default };
