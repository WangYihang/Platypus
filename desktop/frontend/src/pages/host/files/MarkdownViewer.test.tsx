import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wails/go/app/App", () => ({
    ReadFile: vi.fn(),
}));

import { ReadFile } from "@wails/go/app/App";
import MarkdownViewer from "./MarkdownViewer";

function bytesFromString(s: string): number[] {
    return Array.from(new TextEncoder().encode(s));
}

beforeEach(() => {
    vi.mocked(ReadFile).mockReset();
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe("<MarkdownViewer>", () => {
    it("renders Markdown headings and lists from the file bytes", async () => {
        const md = "# Title\n\n- one\n- two\n\nSome **bold** text.\n";
        vi.mocked(ReadFile).mockResolvedValueOnce(bytesFromString(md));

        render(
            <MarkdownViewer
                projectID="p"
                sessionHash="s"
                path="/x/README.md"
                size={md.length}
            />,
        );

        // react-markdown turns # Title into <h1>Title</h1>; assert on
        // the role rather than the literal "#" so we know the parse
        // happened.
        expect(await screen.findByRole("heading", { level: 1, name: /title/i })).toBeInTheDocument();
        expect(screen.getByText("one")).toBeInTheDocument();
        expect(screen.getByText("two")).toBeInTheDocument();
        // Bold text is wrapped in <strong>.
        expect(screen.getByText("bold").tagName.toLowerCase()).toBe("strong");
    });

    it("supports GFM features like task lists", async () => {
        // GFM checkbox lists become input[type=checkbox] only when
        // remark-gfm is wired up.
        const md = "- [x] done\n- [ ] todo\n";
        vi.mocked(ReadFile).mockResolvedValueOnce(bytesFromString(md));

        const { container } = render(
            <MarkdownViewer
                projectID="p"
                sessionHash="s"
                path="/x/TODO.md"
                size={md.length}
            />,
        );

        await screen.findByText(/done/);
        const boxes = container.querySelectorAll('input[type="checkbox"]');
        expect(boxes.length).toBe(2);
        expect((boxes[0] as HTMLInputElement).checked).toBe(true);
        expect((boxes[1] as HTMLInputElement).checked).toBe(false);
    });

    it("surfaces ReadFile errors", async () => {
        vi.mocked(ReadFile).mockRejectedValueOnce(new Error("nope"));

        render(
            <MarkdownViewer
                projectID="p"
                sessionHash="s"
                path="/x/x.md"
                size={1}
            />,
        );

        expect(await screen.findByText(/nope/i)).toBeInTheDocument();
    });
});
