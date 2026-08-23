import { fetchURL, fetchJSON } from "./utils";
import { baseURL } from "@/shared/utils/constants";

export interface TrashItem {
    id: string;
    userID: number;
    originalPath: string;
    trashPath: string;
    name: string;
    deletedAt: string;
    expiresAt: string;
    isDir: boolean;
    size: number;
}

export async function list(): Promise<TrashItem[]> {
    return fetchJSON<TrashItem[]>("/api/trash", {});
}

export async function restore(id: string, destination?: string, override?: boolean): Promise<void> {
    const body = (destination || override) ? JSON.stringify({ destination, override }) : undefined;
    await fetchURL(`/api/trash/${id}/restore`, {
        method: "POST",
        body,
    });
}

export async function remove(id: string): Promise<void> {
    await fetchURL(`/api/trash/${id}`, {
        method: "DELETE",
    });
}

export async function empty(): Promise<void> {
    await fetchURL("/api/trash", {
        method: "DELETE",
    });
}

export const getPreviewURL = (item: TrashItem, size: string) => {
    return `${baseURL}/api/trash/preview/${size}/${item.id}`;
};

export async function updateSettings(retentionDays: number): Promise<void> {
    await fetchURL("/api/trash/settings", {
        method: "PUT",
        body: JSON.stringify({ retentionDays }),
    });
}
