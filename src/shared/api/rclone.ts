import { fetchURL, fetchJSON } from "./utils";

export interface RcloneRemote {
    id: string;
    name: string;
    type: string;
    config: Record<string, string>;
    basePath: string;
    virtualPath: string;
    readOnly: boolean;
    createdAt: string;
    updatedAt?: string;
}

export interface RcloneRemotesResponse {
    remotes: RcloneRemote[];
    backends: string[];
}

export interface CreateRcloneRemoteRequest {
    name: string;
    type: string;
    config: Record<string, string>;
    basePath?: string;
    virtualPath: string;
    readOnly?: boolean;
}

export interface UpdateRcloneRemoteRequest {
    name: string;
    type: string;
    config: Record<string, string>;
    basePath?: string;
    virtualPath: string;
    readOnly?: boolean;
}


export interface BackendsResponse {
    backends: string[];
}

// Get all rclone remotes for a user
export async function listRemotes(userId: number): Promise<RcloneRemotesResponse> {
    return fetchJSON<RcloneRemotesResponse>(`/api/users/${userId}/rclone`, {});
}

// Add a new rclone remote
export async function addRemote(
    userId: number,
    data: CreateRcloneRemoteRequest
): Promise<RcloneRemote> {
    return fetchJSON<RcloneRemote>(`/api/users/${userId}/rclone`, {
        method: "POST",
        body: JSON.stringify(data),
    });
}

// Update a rclone remote
export async function updateRemote(
    userId: number,
    remoteId: string,
    data: UpdateRcloneRemoteRequest
): Promise<void> {
    await fetchURL(`/api/users/${userId}/rclone/${remoteId}`, {
        method: "PUT",
        body: JSON.stringify(data),
    });
}

// Delete a rclone remote
export async function deleteRemote(userId: number, remoteId: string): Promise<void> {
    await fetchURL(`/api/users/${userId}/rclone/${remoteId}`, {
        method: "DELETE",
    });
}

// Test a rclone remote connection
export async function testRemote(
    userId: number,
    remoteId: string
): Promise<string> {
    return fetchJSON<string>(
        `/api/users/${userId}/rclone/${remoteId}/test`,
        { method: "POST" }
    );
}

// Get available rclone backends
export async function getBackends(): Promise<BackendsResponse> {
    return fetchJSON<BackendsResponse>("/api/rclone/backends", {});
}
