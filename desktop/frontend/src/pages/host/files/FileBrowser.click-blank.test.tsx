import { render, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi, beforeEach } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

// Regression: a left-click on a file row used to make the entire file
// listing area collapse — both the listing and the preview pane
// vanished. Two bugs in the previous react-resizable-panels-backed
// implementation stacked on top of each other (one in our shadcn
// wrapper, one in our prop authoring), and the preview-pane mount
// served as the trigger.
//
// The whole panel-group machinery is gone now (replaced by a tiny
// custom <Split>), but the structural invariant still matters: a
// click on a file selects it, opens the preview pane, and KEEPS the
// file grid + every other tile mounted in the DOM. A future
// regression that returns null from the preview branch (or loses the
// file grid via an uncaught render error) would still trip this test.
//
// Mock the Wails App bindings before importing FileBrowser; the
// platform shim normally ships ListDir / ReadFile / etc., we replace
// them with vi.fn so the test exercises pure UI behaviour without
// spinning up an agent.
vi.mock("@wails/go/app/App", () => ({
    ListDir: vi.fn(async () => ({
        entries: [
            { name: "Photos", size: 0, mode: 0o755, modTimeUnix: 0, isDir: true, isSymlink: false },
            { name: "screenshot.png", size: 207 * 1024, mode: 0o644, modTimeUnix: 0, isDir: false, isSymlink: false, mime: "image/png" },
            { name: "find.json", size: 881 * 1024, mode: 0o644, modTimeUnix: 0, isDir: false, isSymlink: false, mime: "application/json" },
        ],
        total: 3,
        eof: true,
    })),
    ReadFile: vi.fn(async () => new Uint8Array([0x89, 0x50, 0x4e, 0x47])),
    WriteFile: vi.fn(async () => undefined),
    DeleteFile: vi.fn(async () => undefined),
    RenameFile: vi.fn(async () => undefined),
    Mkdir: vi.fn(async () => undefined),
    Chmod: vi.fn(async () => undefined),
    DownloadFile: vi.fn(async () => undefined),
    DownloadArchive: vi.fn(async () => undefined),
    UploadFile: vi.fn(async () => undefined),
    PickFileToUpload: vi.fn(async () => ""),
    PickSaveLocation: vi.fn(async () => ""),
}));

vi.mock("../../../components/TransfersPill", () => ({
    useTransfersDrawer: () => ({ setOpen: vi.fn() }),
}));

import FileBrowser from "./FileBrowser";

function renderBrowser() {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
        <QueryClientProvider client={qc}>
            <TooltipProvider>
                <FileBrowser projectID="default" sessionHash="h1" host={null} />
            </TooltipProvider>
        </QueryClientProvider>,
    );
}

beforeEach(() => {
    window.localStorage.clear();
    // Default viewMode is "list" (FileTable); the user's bug report
    // is in grid mode, so seed the preference here.
    window.localStorage.setItem(
        "platypus.pref.ui.files.viewMode",
        JSON.stringify("grid"),
    );
});

describe("FileBrowser click-on-file does not blank the explorer", () => {
    it("a left-click on a file keeps every panel + every tile mounted", async () => {
        const { findByRole, queryByText, container } = renderBrowser();

        // Wait for the directory listing to render.
        const fileTile = await findByRole(
            "button",
            { name: /screenshot\.png/i },
            { timeout: 3000 },
        );
        expect(fileTile).toBeInTheDocument();
        expect(queryByText("Photos")).toBeInTheDocument();
        expect(queryByText("find.json")).toBeInTheDocument();

        fireEvent.click(fileTile);
        await new Promise((r) => setTimeout(r, 100));

        // The preview pane mounts.
        expect(
            container.querySelector('[data-testid="preview-pane"]'),
        ).toBeInTheDocument();
        // Every original tile is still in the DOM. The previewed file
        // is selected; the grid does not unmount.
        expect(
            container.querySelectorAll('button[aria-label*="screenshot.png"]').length,
        ).toBeGreaterThan(0);
        expect(queryByText("Photos")).toBeInTheDocument();
        expect(queryByText("find.json")).toBeInTheDocument();
    });
});
