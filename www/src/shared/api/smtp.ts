import { fetchJSON } from "./utils";

export interface SMTPTestRequest {
    recipient: string;
}

export interface SMTPTestResponse {
    success: boolean;
    error?: string;
    message?: string;
}

// Test SMTP configuration by sending a test email
export async function testSMTP(recipient: string): Promise<string> {
    const req: SMTPTestRequest = { recipient };
    return fetchJSON<string>("/api/settings/test/smtp", {
        method: "POST",
        body: JSON.stringify(req),
    });
}
