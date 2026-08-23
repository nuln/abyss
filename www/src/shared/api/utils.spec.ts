import { describe, expect, it, vi } from "vitest";

// The module under test pulls in app constants which touch `window`;
// stub them for the node test environment.
vi.mock("@/shared/utils/constants", () => ({
    baseURL: "/",
    origin: "http://localhost",
}));

import { removePrefix, StatusError } from "@/shared/api/utils";

describe("removePrefix", () => {
    it("strips the /files view prefix", () => {
        expect(removePrefix("/files/foo/bar.txt")).toBe("/foo/bar.txt");
    });

    it("maps the bare /files listing to root", () => {
        expect(removePrefix("/files")).toBe("/");
    });

    it("normalises empty input to root", () => {
        expect(removePrefix("")).toBe("/");
    });

    it("keeps non-files prefixes such as shared links", () => {
        expect(removePrefix("/shared/me")).toBe("/shared/me");
        expect(removePrefix("/share/abc")).toBe("/share/abc");
    });

    it("ensures a leading slash", () => {
        expect(removePrefix("foo")).toBe("/foo");
    });
});

describe("StatusError", () => {
    it("carries status and cancellation flags", () => {
        const err = new StatusError("000 No connection", 0, true);
        expect(err.name).toBe("StatusError");
        expect(err.status).toBe(0);
        expect(err.is_canceled).toBe(true);
        expect(err.message).toBe("000 No connection");
    });
});
