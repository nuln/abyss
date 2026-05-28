import { fetchURL, fetchJSON } from "./utils";

export type SessionType = "browser" | "client" | "webdav";

export interface Session {
  id: string;
  type: SessionType;
  name: string;
  userAgent: string;
  ip: string;
  tokenId?: number;
  expiresAt?: string;
  createdAt: string;
  lastSeenAt: string;
  canRevoke: boolean;
}

// Get all active sessions for the current user
export async function listSessions(): Promise<Session[]> {
  return fetchJSON<Session[]>("/api/sessions", {});
}

// Revoke a specific session
export async function revokeSession(id: string): Promise<void> {
  await fetchURL(`/api/sessions/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

// Revoke all browser sessions
export async function revokeAllSessions(): Promise<void> {
  await fetchURL("/api/sessions", {
    method: "DELETE",
  });
}
