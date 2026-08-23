import { fetchJSON, fetchURL, StatusError } from "@/shared/api/utils";

export interface DualTokenResponse {
    accessToken: string;
    refreshToken?: string;
    expiresIn: number;
    user?: IUserInfo;
    otp?: boolean;
    token?: string;
}

export interface IUserInfo {
    id: number;
    locale: string;
    viewMode: string;
    singleClick: boolean;
    perm: IPermissions;
    lockPassword: boolean;
    dateFormat: boolean;
    email: string;
    theme: string;
}

export interface IPermissions {
    admin: boolean;
    execute: boolean;
    create: boolean;
    rename: boolean;
    modify: boolean;
    delete: boolean;
    share: boolean;
    download: boolean;
}

export interface LoginRequest {
    email: string;
    username?: string;
    password?: string;
    recaptcha?: string;
}

export interface Session {
    id: string;
    userId: number;
    userAgent: string;
    ip: string;
    createdAt: string;
    isCurrent: boolean;
}

export async function login(request: LoginRequest): Promise<DualTokenResponse> {
    const res = await fetchURL(
        "/api/auth/login",
        {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(request),
        },
        false,
    );

    const data = await res.json();
    return data.success ? data.data : data;
}

export async function refresh(refreshToken: string): Promise<DualTokenResponse | null> {
    try {
        const res = await fetchURL(
            "/api/auth/refresh",
            {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ refreshToken }),
            },
            false,
        );

        const data = await res.json();
        return data.success ? data.data : data;
    } catch {
        return null;
    }
}

export async function logout(refreshToken: string): Promise<void> {
    try {
        await fetchURL(
            "/api/auth/logout",
            {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ refreshToken }),
            },
            false,
        );
    } catch {
        // Ignore logout API failure and continue local cleanup.
    }
}

export async function getSessions(): Promise<Session[]> {
    return fetchURL("/api/auth/sessions", { method: "GET" })
        .then((res) => res.json())
        .then((res) => (res.success ? res.data : res));
}

export async function revokeSession(id: string): Promise<void> {
    await fetchURL(`/api/auth/sessions/${id}`, { method: "DELETE" });
}

export async function revokeAllSessions(): Promise<void> {
    await fetchURL("/api/auth/sessions", { method: "DELETE" });
}

export async function getCurrentUser(): Promise<IUser> {
    return fetchJSON<IUser>("/api/me", { method: "GET" });
}

export { fetchJSON, fetchURL, StatusError };
