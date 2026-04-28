import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./auth", () => ({
    authJSON: vi.fn(),
    getSession: () => ({ serverURL: "https://platypus.example", sessionToken: "tok" }),
}));

import { authJSON } from "./auth";
import { fsReadPreviewURL } from "./fs-preview";

const authJSONMock = vi.mocked(authJSON);

beforeEach(() => {
    authJSONMock.mockReset();
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe("fsReadPreviewURL", () => {
    it("POSTs to /fs/preview-token with the path and returns an absolute URL", async () => {
        // Backend response shape mirrors v2FilePreviewTokenMint:
        //   { token, exp, url } where url is the relative /fs/read URL
        //   with all three signed query params already encoded.
        authJSONMock.mockResolvedValueOnce({
            token: "abc",
            exp: 1700000000,
            url: "/api/v1/projects/p/agents/a/fs/read?path=%2Fmovie.mp4&exp=1700000000&preview_token=abc",
        });

        const url = await fsReadPreviewURL("p", "a", "/movie.mp4");

        const [endpoint, init] = authJSONMock.mock.calls[0];
        expect(endpoint).toBe("/api/v1/projects/p/agents/a/fs/preview-token");
        expect((init as RequestInit).method).toBe("POST");
        expect((init as RequestInit).body).toBe(JSON.stringify({ path: "/movie.mp4" }));
        expect(((init as RequestInit).headers as Record<string, string>)["Content-Type"]).toBe(
            "application/json",
        );

        // Returned URL is what gets dropped into <video src=...> /
        // pdf.js — absolute against the active server origin since
        // the page is usually served from a different origin in web
        // mode.
        expect(url).toBe(
            "https://platypus.example/api/v1/projects/p/agents/a/fs/read?path=%2Fmovie.mp4&exp=1700000000&preview_token=abc",
        );
    });

    it("URL-encodes the project and agent params", async () => {
        // Defensive against future identifier shape changes — pid and
        // aid are UUIDs today but the helper shouldn't assume that.
        authJSONMock.mockResolvedValueOnce({
            token: "t",
            exp: 1,
            url: "/api/v1/projects/p%20one/agents/a%2Fb/fs/read?path=%2Fx&exp=1&preview_token=t",
        });

        await fsReadPreviewURL("p one", "a/b", "/x");

        const [endpoint] = authJSONMock.mock.calls[0];
        expect(endpoint).toBe("/api/v1/projects/p%20one/agents/a%2Fb/fs/preview-token");
    });
});
