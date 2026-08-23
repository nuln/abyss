// import router from "@/app/router";
import { useAuthStore } from "@/domains/auth";
import {
    getCurrentUser,
    login as loginAPI,
    logout as logoutAPI,
    refresh,
    StatusError,
} from "@/domains/auth/api";
import type { JwtPayload } from "jwt-decode";
import { jwtDecode } from "jwt-decode";
import { noAuth, logoutPage, baseURL } from "@/shared/utils/constants";
import { setSafeTimeout } from "@/shared/api/utils";

const ACCESS_TOKEN_KEY = "accessToken";
const REFRESH_TOKEN_KEY = "refreshToken";

interface AccessTokenClaims extends JwtPayload {
    uid?: number;
    email?: string;
    ctype?: string;
    user?: IUser;
}

const cookieSecure = window.location.protocol === "https:" ? "; Secure" : "";

export function storeAccessToken(token: string) {
    document.cookie = `auth=${token}; Path=/; SameSite=Strict;${cookieSecure}`;
    localStorage.setItem(ACCESS_TOKEN_KEY, token);

    const authStore = useAuthStore();
    authStore.jwt = token;

    try {
        const data = jwtDecode<JwtPayload>(token);
        if (data.exp) {
            if (authStore.logoutTimer) {
                clearTimeout(authStore.logoutTimer);
            }
            const expiresAt = new Date(data.exp * 1000);
            const timeout = expiresAt.getTime() - Date.now();
            const refreshTimeout = timeout - 60000;
            if (refreshTimeout > 0) {
                authStore.setLogoutTimer(
                    setSafeTimeout(() => {
                        void refreshAccessToken();
                    }, refreshTimeout),
                );
            }
        }
    } catch {
        // Ignore decode failures here.
    }
}

export function storeRefreshToken(token: string) {
    localStorage.setItem(REFRESH_TOKEN_KEY, token);
}

export function getAccessToken(): string | null {
    return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
    return localStorage.getItem(REFRESH_TOKEN_KEY);
}

function clearTokens() {
    document.cookie = `auth=; Max-Age=0; Path=/; SameSite=Strict;${cookieSecure}`;
    localStorage.removeItem(ACCESS_TOKEN_KEY);
    localStorage.removeItem(REFRESH_TOKEN_KEY);
    localStorage.removeItem("jwt");
}

let refreshInFlight: Promise<boolean> | null = null;

// Single-flight wrapper: concurrent callers (multiple tabs/timers hitting
// the expiry window together) share one refresh request.
export function refreshAccessToken(): Promise<boolean> {
    if (!refreshInFlight) {
        refreshInFlight = doRefreshAccessToken().finally(() => {
            refreshInFlight = null;
        });
    }
    return refreshInFlight;
}

async function doRefreshAccessToken(): Promise<boolean> {
    const refreshToken = getRefreshToken();
    if (!refreshToken) {
        return false;
    }

    try {
        const data = await refresh(refreshToken);
        if (data && data.accessToken) {
            storeAccessToken(data.accessToken);
            return true;
        }
        clearTokens();
        return false;
    } catch {
        clearTokens();
        return false;
    }
}

export function parseToken(token: string) {
    const data = jwtDecode<AccessTokenClaims>(token);

    localStorage.setItem("jwt", token);
    // storeAccessToken writes the cookie/store and schedules the silent
    // refresh; do NOT arm a hard logout at expiry — that would defeat the
    // dual-token design and kick users out mid-session.
    storeAccessToken(token);

    if (data.user) {
        useAuthStore().setUser(data.user as IUser);
    } else if (data.uid) {
        void getCurrentUser()
            .then((user) => {
                useAuthStore().setUser(user);
            })
            .catch(() => {
                // Ignore profile hydration failures.
            });
    }
}

export async function validateLogin() {
    const authStore = useAuthStore();

    const accessToken = getAccessToken();
    if (accessToken) {
        try {
            const data = jwtDecode<AccessTokenClaims>(accessToken);
            if (data.exp && data.exp * 1000 > Date.now()) {
                storeAccessToken(accessToken);

                if (!authStore.user) {
                    if (data.user) {
                        authStore.setUser(data.user);
                    } else if (data.uid) {
                        try {
                            const user = await getCurrentUser();
                            authStore.setUser(user);
                        } catch {
                            // Ignore profile fetch failures.
                        }
                    }
                }
                return;
            }

            if (await refreshAccessToken()) {
                const newToken = getAccessToken();
                if (newToken) {
                    const newData = jwtDecode<AccessTokenClaims>(newToken);
                    if (!authStore.user && newData.uid) {
                        try {
                            const user = await getCurrentUser();
                            authStore.setUser(user);
                        } catch {
                            // Ignore profile fetch failures.
                        }
                    }
                }
                return;
            }
        } catch {
            // Continue fallback checks.
        }
    }

    const jwt = localStorage.getItem("jwt");
    if (jwt) {
        try {
            parseToken(jwt);
            if (!authStore.user) {
                const data = jwtDecode<AccessTokenClaims>(jwt);
                if (data.uid) {
                    try {
                        const user = await getCurrentUser();
                        authStore.setUser(user);
                    } catch {
                        // Ignore profile fetch failures.
                    }
                }
            }
            return;
        } catch (error) {
            localStorage.removeItem("jwt");
            throw error;
        }
    }

    throw new Error("No valid token");
}

export async function login(email: string, password: string, recaptcha: string): Promise<{ otp: boolean; token: string }> {
    try {
        const payload = await loginAPI({ email, password, recaptcha });

        // Handle both legacy `otp` field and new MFA flow (`needMFA` + `mfaToken`)
        const needsMFA = payload.otp || (payload as any).needMFA;
        const mfaToken = (payload as any).mfaToken || payload.token || "";
        if (needsMFA) {
            return { otp: true, token: mfaToken };
        }

        storeAccessToken(payload.accessToken);
        if (payload.refreshToken) {
            storeRefreshToken(payload.refreshToken);
        }
        if (payload.user) {
            useAuthStore().setUser(payload.user as any);
        } else {
            try {
                const user = await getCurrentUser();
                useAuthStore().setUser(user);
            } catch {
                // Ignore profile hydration failures.
            }
        }

        return { otp: false, token: payload.accessToken };
    } catch (e: any) {
        throw new StatusError(e.message || "Login failed", e.status);
    }
}

export async function signup(email: string, username: string, password: string) {
    const res = await fetch(`${baseURL}/api/signup`, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify({ email, username, password }),
    });

    if (!res.ok) {
        const body = await res.text();
        throw new StatusError(body || `${res.status} ${res.statusText}`, res.status);
    }
}

export async function logout(reason?: string) {
    const refreshToken = getRefreshToken();
    if (refreshToken) {
        await logoutAPI(refreshToken);
    }

    clearTokens();

    const authStore = useAuthStore();
    authStore.clearUser();

    if (noAuth) {
        window.location.reload();
    } else if (logoutPage !== "/login") {
        document.location.href = `${logoutPage}`;
    } else if (typeof reason === "string" && reason.trim() !== "") {
        window.location.href = `${baseURL}/login?logout-reason=${encodeURIComponent(reason)}`;
    } else {
        window.location.href = `${baseURL}/login`;
    }
}
