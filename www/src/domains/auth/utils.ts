// Authentication utilities — cookie-based session mode.
//
// Since phase 2 the server issues HttpOnly cookies (abyss_at / abyss_rt) on
// login, MFA verification and refresh. The SPA never touches the tokens:
// it only tracks the user object and schedules a silent refresh shortly
// before the access token expires, using the metadata returned by the
// login/refresh endpoints.

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
import { baseURL, noAuth, logoutPage } from "@/shared/utils/constants";

const LEGACY_KEYS = ["jwt", "accessToken", "refreshToken"];

function clearLegacyStorage() {
    // Tokens used to live in localStorage; scrub any leftovers from the
    // pre-cookie era so stale credentials can never be resurrected.
    for (const key of LEGACY_KEYS) {
        localStorage.removeItem(key);
    }
    // The legacy JS-managed auth cookie is superseded by the HttpOnly one.
    document.cookie = "auth=; Max-Age=0; Path=/; SameSite=Strict;";
}

export function getAccessToken(): string | null {
    // Kept for API compatibility; the HttpOnly cookie is not readable here.
    return null;
}

async function fetchCurrentUser(): Promise<IUser | null> {
    try {
        return await getCurrentUser();
    } catch {
        return null;
    }
}

/** Silent refresh: the refresh token rides in its HttpOnly cookie. */
export async function refreshAccessToken(): Promise<boolean> {
    try {
        const data = await refresh("");
        if (data && data.accessToken) {
            scheduleSilentRefresh(data.accessToken);
            return true;
        }
        return false;
    } catch {
        return false;
    }
}

let silentRefreshTimer: number | null = null;

function clearSilentRefresh() {
    if (silentRefreshTimer !== null) {
        clearTimeout(silentRefreshTimer);
        silentRefreshTimer = null;
    }
}

function setSilentRefreshTimer(timer: number) {
    clearSilentRefresh();
    silentRefreshTimer = timer;
}

export function setLogoutTimer(_timer: number) {
    // Deprecated no-op kept for store compatibility; the silent-refresh
    // timer replaces the old hard-logout-at-expiry behaviour.
}

// Schedule a refresh ~60s before the access token in `token` expires.
function scheduleSilentRefresh(token?: string) {
    if (!token) return;
    try {
        const data = jwtDecode<JwtPayload>(token);
        if (!data.exp) return;
        const delay = data.exp * 1000 - Date.now() - 60_000;
        if (delay <= 0) return;
        setSilentRefreshTimer(
            window.setTimeout(() => {
                void refreshAccessToken();
            }, delay),
        );
    } catch {
        // Ignore decode failures.
    }
}

/**
 * Restore a session on page load. Order of attempts:
 *  1. valid access cookie  -> GET /api/me
 *  2. refresh cookie       -> POST /api/auth/refresh, then /api/me
 *  3. legacy localStorage tokens from the pre-cookie era
 */
export async function validateLogin() {
    const authStore = useAuthStore();

    const user = await fetchCurrentUser();
    if (user) {
        clearLegacyStorage();
        authStore.setUser(user);
        // Re-arm the silent-refresh chain: the exp is only readable from
        // the token itself, which the SPA can no longer inspect, so do one
        // lightweight refresh now — it schedules all subsequent ones.
        await refreshAccessToken();
        return;
    }

    if (await refreshAccessToken()) {
        const refreshed = await fetchCurrentUser();
        if (refreshed) {
            clearLegacyStorage();
            authStore.setUser(refreshed);
            return;
        }
    }

    // Legacy path: tokens left over in localStorage from an older build.
    const accessToken = localStorage.getItem("accessToken");
    if (accessToken) {
        try {
            const data = jwtDecode<JwtPayload>(accessToken);
            if (data.exp && data.exp * 1000 > Date.now()) {
                // Still works against the X-Auth channel until the user
                // logs in again and migrates to cookies.
                document.cookie = `auth=${accessToken}; Path=/; SameSite=Strict;`;
                const legacyUser = await fetchCurrentUser();
                if (legacyUser) {
                    authStore.setUser(legacyUser);
                    scheduleSilentRefresh(accessToken);
                    return;
                }
            }
        } catch {
            // Fall through to failure below.
        }
    }

    clearLegacyStorage();
    throw new Error("No valid token");
}

// ── Plugin SDK compatibility shims ──────────────────────────────────────────
// Tokens are managed by HttpOnly cookies server-side. These keep the
// plugin auth API signature stable: parseToken only needs to hydrate the
// user profile (the cookie was already set by the issuing endpoint), and
// storing a refresh token is a no-op.

export function parseToken(token: string) {
    void token;
    void getCurrentUser()
        .then((user) => {
            useAuthStore().setUser(user);
        })
        .catch(() => {
            // Ignore profile hydration failures.
        });
}

export function storeRefreshToken(_token: string) {
    // no-op: the refresh token lives in an HttpOnly cookie.
}

export async function login(email: string, password: string, recaptcha: string): Promise<{ otp: boolean; token: string }> {    try {
        const payload = await loginAPI({ email, password, recaptcha });

        // Handle both the legacy `otp` field and the MFA flow (`needMFA` +
        // `mfaToken`). The intermediate MFA token is short-lived and stays
        // in memory only.
        const needsMFA = payload.otp || (payload as any).needMFA;
        if (needsMFA) {
            const mfaToken = (payload as any).mfaToken || payload.token || "";
            return { otp: true, token: mfaToken };
        }

        // Cookies are already set by the server. Hydrate the user profile.
        clearLegacyStorage();
        if (payload.user) {
            useAuthStore().setUser(payload.user as IUser);
        } else {
            const user = await getCurrentUser();
            useAuthStore().setUser(user);
        }
        scheduleSilentRefresh(payload.accessToken);

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
    // Server clears the HttpOnly cookies and revokes the refresh session.
    try {
        await logoutAPI("");
    } catch {
        // Ignore logout API failure and continue local cleanup.
    }
    clearLegacyStorage();
    clearSilentRefresh();

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
