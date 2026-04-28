import { render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wails/go/app/App", () => ({
    ReadFile: vi.fn(),
}));

vi.mock("@/lib/fs-preview", () => ({
    fsReadPreviewURL: vi.fn(),
}));

import { ReadFile } from "@wails/go/app/App";
import { fsReadPreviewURL } from "@/lib/fs-preview";
import MediaViewer from "./MediaViewer";

const BYTES = [0, 1, 2, 3];

beforeEach(() => {
    vi.spyOn(URL, "createObjectURL").mockImplementation((obj: Blob | MediaSource) => {
        const t = obj instanceof Blob ? obj.type : "media";
        return `blob:test/media-${t}`;
    });
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
});

afterEach(() => {
    vi.mocked(ReadFile).mockReset();
    vi.mocked(fsReadPreviewURL).mockReset();
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
});

describe("<MediaViewer> (desktop / blob-URL fallback)", () => {
    it("renders <video> with controls for video kinds", async () => {
        vi.mocked(ReadFile).mockResolvedValueOnce(BYTES);

        const { container } = render(
            <MediaViewer
                projectID="p"
                sessionHash="s"
                path="/x/clip.mp4"
                size={4}
                kind="video"
                mime="video/mp4"
            />,
        );

        await waitFor(() => {
            const v = container.querySelector("video");
            expect(v).not.toBeNull();
            expect(v?.getAttribute("controls")).not.toBeNull();
            expect(v?.getAttribute("src")).toContain("video/mp4");
        });
    });

    it("renders <audio> with controls for audio kinds", async () => {
        vi.mocked(ReadFile).mockResolvedValueOnce(BYTES);

        const { container } = render(
            <MediaViewer
                projectID="p"
                sessionHash="s"
                path="/x/song.mp3"
                size={4}
                kind="audio"
                mime="audio/mpeg"
            />,
        );

        await waitFor(() => {
            const a = container.querySelector("audio");
            expect(a).not.toBeNull();
            expect(a?.getAttribute("controls")).not.toBeNull();
            expect(a?.getAttribute("src")).toContain("audio/mpeg");
        });
    });

    it("shows a load error when ReadFile fails", async () => {
        vi.mocked(ReadFile).mockRejectedValueOnce(new Error("nope"));

        const { findByText } = render(
            <MediaViewer
                projectID="p"
                sessionHash="s"
                path="/x/x.mp4"
                size={1}
                kind="video"
            />,
        );

        await findByText(/nope/i);
    });
});

// Web mode swaps the blob-URL load for a server-minted preview URL.
// The browser-native <video>/<audio> can then issue Range requests
// against /fs/read directly — the whole point of this Phase. We assert
// the native element receives the preview URL and that the legacy
// ReadFile pipe is bypassed entirely.
describe("<MediaViewer> (web / preview-URL path)", () => {
    beforeEach(() => {
        vi.stubEnv("MODE", "web");
    });

    it("renders <video src=...> with the preview URL and skips ReadFile", async () => {
        vi.mocked(fsReadPreviewURL).mockResolvedValueOnce(
            "https://platypus.example/api/v1/projects/p/agents/a/fs/read?path=%2Fx%2Fclip.mp4&exp=1&preview_token=t",
        );

        const { container } = render(
            <MediaViewer
                projectID="p"
                sessionHash="a"
                path="/x/clip.mp4"
                size={4}
                kind="video"
                mime="video/mp4"
            />,
        );

        await waitFor(() => {
            const v = container.querySelector("video");
            expect(v).not.toBeNull();
            expect(v?.getAttribute("src")).toBe(
                "https://platypus.example/api/v1/projects/p/agents/a/fs/read?path=%2Fx%2Fclip.mp4&exp=1&preview_token=t",
            );
            // Native preload="metadata" pulls just the file header so
            // duration / dimensions render without downloading the
            // whole payload — the Range pipeline does the rest as the
            // user seeks.
            expect(v?.getAttribute("preload")).toBe("metadata");
        });

        expect(ReadFile).not.toHaveBeenCalled();
        expect(fsReadPreviewURL).toHaveBeenCalledWith("p", "a", "/x/clip.mp4");
    });

    it("renders <audio src=...> with the preview URL", async () => {
        vi.mocked(fsReadPreviewURL).mockResolvedValueOnce(
            "https://platypus.example/api/v1/projects/p/agents/a/fs/read?path=%2Fsong.mp3&exp=1&preview_token=t",
        );

        const { container } = render(
            <MediaViewer
                projectID="p"
                sessionHash="a"
                path="/song.mp3"
                size={4}
                kind="audio"
                mime="audio/mpeg"
            />,
        );

        await waitFor(() => {
            const a = container.querySelector("audio");
            expect(a?.getAttribute("src")).toContain("preview_token=");
        });
        expect(ReadFile).not.toHaveBeenCalled();
    });

    it("surfaces a mint failure as a load error", async () => {
        vi.mocked(fsReadPreviewURL).mockRejectedValueOnce(new Error("mint failed"));

        const { findByText } = render(
            <MediaViewer
                projectID="p"
                sessionHash="a"
                path="/x.mp4"
                size={1}
                kind="video"
            />,
        );

        await findByText(/mint failed/i);
    });
});
