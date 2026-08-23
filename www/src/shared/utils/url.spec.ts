import { describe, expect, it } from "vitest";
import {
    encodePath,
    encodeRFC5987ValueChars,
    removeLastDir,
} from "@/shared/utils/url";

describe("encodePath", () => {
    it("keeps slashes as separators", () => {
        expect(encodePath("a/b")).toBe("a/b");
        expect(encodePath("/a/b/c")).toBe("/a/b/c");
    });

    it("encodes special characters within segments only", () => {
        expect(encodePath("my file/report #1?.txt")).toBe(
            "my%20file/report%20%231%3F.txt"
        );
        expect(encodePath("中文/文件夹")).toBe(
            encodeURIComponent("中文") + "/" + encodeURIComponent("文件夹")
        );
    });

    it("does not double-encode safe characters", () => {
        expect(encodePath("a.b-c_d~e")).toBe("a.b-c_d~e");
    });
});

describe("removeLastDir", () => {
    it("removes the last path segment with trailing slash", () => {
        expect(removeLastDir("/a/b/")).toBe("/a");
        expect(removeLastDir("/files/sub/dir/")).toBe("/files/sub");
    });

    it("removes the last path segment without trailing slash", () => {
        expect(removeLastDir("/a/b")).toBe("/a");
    });

    it("collapses root to empty string", () => {
        expect(removeLastDir("/")).toBe("");
    });
});

describe("encodeRFC5987ValueChars", () => {
    it("encodes reserved mark characters with uppercase hex", () => {
        expect(encodeRFC5987ValueChars("*")).toBe("%2A");
        expect(encodeRFC5987ValueChars("'")).toBe("%27");
        expect(encodeRFC5987ValueChars("(")).toBe("%28");
        expect(encodeRFC5987ValueChars(")")).toBe("%29");
    });

    it("keeps RFC5987-allowed delimiters readable", () => {
        expect(encodeRFC5987ValueChars("a|b`c^d")).toBe("a|b`c^d");
    });

    it("encodes non-ASCII as UTF-8 percent sequences", () => {
        expect(encodeRFC5987ValueChars("中")).toBe("%E4%B8%AD");
    });
});
