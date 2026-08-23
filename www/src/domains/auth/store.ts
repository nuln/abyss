import { defineStore } from "pinia";
import { detectLocale, setLocale } from "@/shared/i18n";
import { cloneDeep } from "lodash-es";
import { fetchJSON } from "@/domains/auth/api";

export const useAuthStore = defineStore("auth", {
    state: (): {
        user: IUser | null;
        logoutTimer: number | null;
        initialized: boolean;
    } => ({
        user: null,
        logoutTimer: null,
        initialized: true,
    }),
    getters: {
        isLoggedIn: (state) => state.user !== null,
        needsSetup: (state) => !state.initialized,
    },
    actions: {
        async checkSetup() {
            try {
                const data = await fetchJSON<{ initialized: boolean }>("/api/setup/status");
                this.initialized = data.initialized;
            } catch (_e) {
                this.initialized = true;
            }
        },
        setUser(user: IUser | null) {
            if (!user) {
                this.user = null;
                return;
            }

            setLocale(user.locale || detectLocale());
            this.user = user;
        },
        updateUser(user: Partial<IUser>) {
            if (user.locale) {
                setLocale(user.locale);
            }

            this.user = { ...this.user, ...cloneDeep(user) } as IUser;
        },
        clearUser() {
            this.$reset();
        },
        setLogoutTimer(logoutTimer: number | null) {
            this.logoutTimer = logoutTimer;
        },
    },
});
