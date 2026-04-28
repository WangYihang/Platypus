import { render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wails/go/app/App", () => ({
    ReadFile: vi.fn(),
}));

import { ReadFile } from "@wails/go/app/App";
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
    vi.restoreAllMocks();
});

describe("<MediaViewer>", () => {
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
