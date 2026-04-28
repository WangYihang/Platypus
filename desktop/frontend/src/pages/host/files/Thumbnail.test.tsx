import { render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wails/go/app/App", () => ({
    ReadFile: vi.fn(),
}));

import { ReadFile } from "@wails/go/app/App";
import Thumbnail from "./Thumbnail";

const PNG_BYTES = [137, 80, 78, 71];

beforeEach(() => {
    vi.spyOn(URL, "createObjectURL").mockImplementation((obj: Blob | MediaSource) => {
        const type = obj instanceof Blob ? obj.type : "media";
        return `blob:test-thumb/${type}`;
    });
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
});

afterEach(() => {
    vi.mocked(ReadFile).mockReset();
    vi.restoreAllMocks();
});

describe("<Thumbnail>", () => {
    it("loads the image lazily once visible and renders <img>", async () => {
        vi.mocked(ReadFile).mockResolvedValueOnce(PNG_BYTES);

        const { container } = render(
            <Thumbnail
                projectID="p"
                sessionHash="s"
                path="/x/logo.png"
                mime="image/png"
            />,
        );

        // Decorative alt="" intentionally excludes the img from the
        // a11y tree, so query the DOM directly.
        await waitFor(() => {
            const img = container.querySelector("img");
            expect(img).not.toBeNull();
            expect((img as HTMLImageElement).src).toContain("image/png");
        });
        expect(ReadFile).toHaveBeenCalledWith("p", "s", "/x/logo.png", 0, 0);
    });

    it("does not crash when the read fails — falls back silently", async () => {
        vi.mocked(ReadFile).mockRejectedValueOnce(new Error("denied"));

        const { container } = render(
            <Thumbnail
                projectID="p"
                sessionHash="s"
                path="/x/bad.png"
                mime="image/png"
            />,
        );

        await waitFor(() => {
            expect(ReadFile).toHaveBeenCalled();
        });
        // No <img> rendered on failure; the component stays mounted
        // with the placeholder icon so tile dimensions stay stable.
        expect(container.querySelector("img")).toBeNull();
    });
});
