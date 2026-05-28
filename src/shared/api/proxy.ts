export interface ProxyTestRequest {
    testURL: string;
}

import { fetchJSON } from "./utils";

export interface ProxyTestResponse {
    success: boolean;
    error?: string;
    message?: string;
}

// Test proxy configuration
export async function testProxy(testURL: string): Promise<ProxyTestResponse> {
    const req: ProxyTestRequest = { testURL };
    return fetchJSON<ProxyTestResponse>("/api/settings/test/proxy", {
        method: "POST",
        body: JSON.stringify(req),
    });
}
