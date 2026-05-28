import { fetchURL, fetchJSON, StatusError } from "@/shared/api/utils";

export const settings = {
    get() {
        return fetchJSON<ISettings>("/api/settings", {});
    },
    async update(data: ISettings) {
        await fetchURL("/api/settings", {
            method: "PUT",
            body: JSON.stringify(data),
        });
    },
    async runGC(dryRun = false, force = true) {
        return fetchJSON<{ deletedSize: number; deletedCount: number }>(
            `/api/settings/gc?dryRun=${dryRun}&force=${force}`,
            { method: "POST" },
        );
    },
    async getGCStatus() {
        return fetchJSON<{ running: boolean; history: GCHistory[] }>("/api/settings/gc", {
            method: "GET",
        });
    },
    async migrateStorage(type: string) {
        return fetchJSON<MigrationStatus>(
            `/api/settings/storage/migrate?type=${type}`,
            { method: "POST" },
        );
    },
    async getMigrationStatus() {
        return fetchJSON<MigrationStatus>("/api/settings/storage/migrate", {
            method: "GET",
        });
    },
    async preflightMigration(type: string) {
        return fetchJSON<PreflightResult>(
            `/api/settings/storage/migrate/preflight?type=${encodeURIComponent(type)}`,
            { method: "GET" },
        );
    },
};

export const users = {
    async getAll() {
        return fetchJSON<IUser[]>("/api/users", {});
    },
    async get(id: number) {
        return fetchJSON<IUser>(`/api/users/${id}`, {});
    },
    async create(user: IUser) {
        const res = await fetchURL("/api/users", {
            method: "POST",
            body: JSON.stringify({
                what: "user",
                which: [],
                data: user,
            }),
        });

        if (res.status === 201) {
            return res.headers.get("Location");
        }

        throw new StatusError(await res.text(), res.status);
    },
    async update(user: Partial<IUser>, which = ["all"], currentPassword?: string, auth = true) {
        await fetchURL(
            `/api/users/${user.id}`,
            {
                method: "PUT",
                body: JSON.stringify({
                    what: "user",
                    which,
                    data: user,
                    currentPassword,
                }),
            },
            auth,
        );
    },
    async remove(id: number) {
        await fetchURL(`/api/users/${id}`, { method: "DELETE" });
    },
    async enableOTP(id: number, password: string) {
        return fetchJSON<IOtpSetupKey>(`/api/users/${id}/otp`, {
            method: "POST",
            body: JSON.stringify({ password }),
        });
    },
    async checkOtp(id: number, code: string) {
        return fetchURL(`/api/users/${id}/otp/check`, {
            method: "POST",
            body: JSON.stringify({ code }),
        });
    },
    async getOtpInfo(id: number, code: string) {
        return fetchJSON<IOtpSetupKey>(`/api/users/${id}/otp`, {
            method: "GET",
            headers: { "X-TOTP-CODE": code },
        });
    },
    async disableOtp(id: number, code: string) {
        return fetchURL(`/api/users/${id}/otp`, {
            method: "DELETE",
            headers: { "X-TOTP-CODE": code },
        });
    },
    async resetOtp(id: number, password: string) {
        return fetchJSON<IOtpSetupKey>(`/api/users/${id}/otp/action?type=reset`, {
            method: "POST",
            body: JSON.stringify({ password }),
        });
    },
    async generateRecoveryCodes(id: number, code: string) {
        return fetchJSON<{ codes: string[] }>(`/api/users/${id}/otp/action?type=recovery`, {
            method: "POST",
            headers: { "X-TOTP-CODE": code },
        });
    },
    async toggleOtp(id: number, enabled: boolean) {
        return fetchURL(`/api/users/${id}/otp/action?type=toggle`, {
            method: "POST",
            body: JSON.stringify({ enabled }),
        });
    },
    async listPasskeys(id: number) {
        return fetchJSON<IPasskeyListResponse>(`/api/users/${id}/passkey`, {});
    },
    async beginPasskeyRegistration(id: number) {
        return fetchJSON<PublicKeyCredentialCreationOptions>(`/api/users/${id}/passkey/begin`, {});
    },
    async finishPasskeyRegistration(id: number, credential: Credential, name: string) {
        const pkCred = credential as PublicKeyCredential;
        const response = pkCred.response as AuthenticatorAttestationResponse;

        const body = {
            id: pkCred.id,
            rawId: arrayBufferToBase64URL(pkCred.rawId),
            type: pkCred.type,
            response: {
                clientDataJSON: arrayBufferToBase64URL(response.clientDataJSON),
                attestationObject: arrayBufferToBase64URL(response.attestationObject),
            },
            name,
        };

        return fetchURL(`/api/users/${id}/passkey/finish`, {
            method: "POST",
            body: JSON.stringify(body),
        });
    },
    async deletePasskey(id: number, credentialId: string) {
        return fetchURL(`/api/users/${id}/passkey/${credentialId}`, {
            method: "DELETE",
        });
    },
    async togglePasskeyCredential(id: number, credentialId: string, enabled: boolean) {
        return fetchURL(`/api/users/${id}/passkey/${credentialId}/toggle`, {
            method: "PUT",
            body: JSON.stringify({ enabled }),
        });
    },
    async togglePasskey(id: number, enabled: boolean) {
        return fetchURL(`/api/users/${id}/passkey/toggle`, {
            method: "PUT",
            body: JSON.stringify({ enabled }),
        });
    },
};

function arrayBufferToBase64URL(buffer: ArrayBuffer): string {
    const bytes = new Uint8Array(buffer);
    let binary = "";
    for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
}

export { fetchJSON, StatusError };
