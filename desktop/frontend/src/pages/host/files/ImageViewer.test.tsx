import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wails/go/app/App", () => ({
    ReadFile: vi.fn(),
}));

import { ReadFile } from "@wails/go/app/App";
import ImageViewer from "./ImageViewer";

// jsdom doesn't ship a real URL.createObjectURL — stub a deterministic
// implementation so the rendered <img src> is assertable.
const createdURLs: string[] = [];
const revokedURLs: string[] = [];

beforeEach(() => {
    createdURLs.length = 0;
    revokedURLs.length = 0;
    vi.spyOn(URL, "createObjectURL").mockImplementation((obj: Blob | MediaSource) => {
        const type = obj instanceof Blob ? obj.type : "media";
        const url = `blob:test/${createdURLs.length}-${type}`;
        createdURLs.push(url);
        return url;
    });
    vi.spyOn(URL, "revokeObjectURL").mockImplementation((url: string) => {
        revokedURLs.push(url);
    });
});

afterEach(() => {
    vi.mocked(ReadFile).mockReset();
    vi.restoreAllMocks();
});

const PNG_BYTES = [137, 80, 78, 71]; // \x89PNG marker; payload itself is irrelevant for the test.

describe("<ImageViewer>", () => {
    it("loads bytes from ReadFile and renders an <img> with a blob URL", async () => {
        vi.mocked(ReadFile).mockResolvedValueOnce(PNG_BYTES);

        render(
            <ImageViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/logo.png"
                size={PNG_BYTES.length}
                mime="image/png"
            />,
        );

        const img = await screen.findByRole("img");
        expect(img).toBeInTheDocument();
        expect((img as HTMLImageElement).src).toBe(createdURLs[0]);
        expect(createdURLs[0]).toContain("image/png");
        expect(ReadFile).toHaveBeenCalledWith("p", "s", "/tmp/logo.png", 0, 0);
    });

    it("falls back to image/* mime when none is supplied", async () => {
        // No mime — viewer should still produce a usable blob URL via
        // a generic image type so the browser doesn't refuse to render.
        vi.mocked(ReadFile).mockResolvedValueOnce(PNG_BYTES);

        render(
            <ImageViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/x.png"
                size={PNG_BYTES.length}
            />,
        );

        await screen.findByRole("img");
        expect(createdURLs[0]).toMatch(/^blob:test\/0-/);
    });

    it("shows a load error when ReadFile rejects", async () => {
        vi.mocked(ReadFile).mockRejectedValueOnce(new Error("boom"));

        render(
            <ImageViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/bad.png"
                size={4}
                mime="image/png"
            />,
        );

        await waitFor(() => expect(screen.getByText(/boom/i)).toBeInTheDocument());
        expect(screen.queryByRole("img")).toBeNull();
    });
});
