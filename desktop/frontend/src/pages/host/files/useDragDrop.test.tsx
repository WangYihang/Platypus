import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, waitFor } from "@testing-library/react";

// Wails EventsOn registry: capture handlers so tests can synthesise
// the desktop OS-drop event without going through the real bridge.
const wailsListeners = new Map<string, (payload: unknown) => void>();

vi.mock("@wails/runtime/runtime", () => ({
    EventsOn: (name: string, fn: (payload: unknown) => void) => {
        wailsListeners.set(name, fn);
    },
    EventsOff: (name: string) => {
        wailsListeners.delete(name);
    },
}));

// Both UploadFile and UploadBrowserFile resolve to src/platform/App.web.ts
// under the vitest alias map (see vitest.config.ts), so we mock that
// module once with both names.
const uploadFileMock = vi.fn().mockResolvedValue(undefined);
const uploadBrowserFileMock = vi.fn().mockResolvedValue(undefined);
vi.mock("@/platform/App.web", () => ({
    UploadFile: (...args: unknown[]) => uploadFileMock(...args),
    UploadBrowserFile: (...args: unknown[]) => uploadBrowserFileMock(...args),
}));

import { useDragDrop } from "./useDragDrop";

beforeEach(() => {
    wailsListeners.clear();
    uploadFileMock.mockClear();
    uploadBrowserFileMock.mockClear();
});

// Test harness: a tiny component that wires the hook's drop handlers
// onto a div the test can fireEvent against.
function Harness({
    onStart,
    onFinished,
    onError,
    onProgress,
}: {
    onStart?: () => void;
    onFinished?: () => void;
    onError?: (e: string) => void;
    onProgress?: (p: { filename: string; done: number; total: number; error?: string }) => void;
}) {
    const { dropHandlers } = useDragDrop({
        projectID: "p1",
        sessionHash: "h1",
        currentPath: "/tmp",
        onStart,
        onFinished,
        onError,
        onProgress,
    });
    return (
        <div data-testid="zone" {...dropHandlers}>
            drop here
        </div>
    );
}

// Phase 3 contract: every entry path that starts an upload must
// notify its caller via `onStart` so wiring it up to "open the
// transfers drawer" is uniform across the upload button + drag-drop.
describe("useDragDrop onStart hook", () => {
    it("calls onStart exactly once before UploadBrowserFile when one file is dropped (web)", async () => {
        const onStart = vi.fn();
        const { getByTestId } = render(<Harness onStart={onStart} />);
        const zone = getByTestId("zone");

        const file = new File(["alpha"], "a.txt", { type: "text/plain" });
        fireEvent.drop(zone, { dataTransfer: { files: [file] } });

        await waitFor(() => expect(uploadBrowserFileMock).toHaveBeenCalledTimes(1));
        expect(onStart).toHaveBeenCalledTimes(1);
        // onStart fires before the upload promise — guarantee by
        // recording call orders against a shared counter.
        const startOrder = onStart.mock.invocationCallOrder[0];
        const uploadOrder = uploadBrowserFileMock.mock.invocationCallOrder[0];
        expect(startOrder).toBeLessThan(uploadOrder);
    });

    it("calls onStart exactly once across a multi-file drop (web)", async () => {
        const onStart = vi.fn();
        const { getByTestId } = render(<Harness onStart={onStart} />);
        const zone = getByTestId("zone");

        const f1 = new File(["a"], "a.txt");
        const f2 = new File(["b"], "b.txt");
        const f3 = new File(["c"], "c.txt");
        fireEvent.drop(zone, { dataTransfer: { files: [f1, f2, f3] } });

        await waitFor(() => expect(uploadBrowserFileMock).toHaveBeenCalledTimes(3));
        expect(onStart).toHaveBeenCalledTimes(1);
    });

    it("does NOT call onStart when the drop carries no files (web)", async () => {
        const onStart = vi.fn();
        const { getByTestId } = render(<Harness onStart={onStart} />);
        const zone = getByTestId("zone");

        fireEvent.drop(zone, { dataTransfer: { files: [] } });

        // Give the microtask queue a tick so any spurious async path
        // would've fired by now.
        await Promise.resolve();
        expect(onStart).not.toHaveBeenCalled();
        expect(uploadBrowserFileMock).not.toHaveBeenCalled();
    });

    it("calls onStart once before UploadFile on the desktop OS-drop branch", async () => {
        const onStart = vi.fn();
        render(<Harness onStart={onStart} />);

        // The hook subscribed to "files:os-drop" — invoke its handler
        // with two paths to simulate Wails delivering an OS drop event.
        const handler = wailsListeners.get("files:os-drop");
        expect(handler).toBeDefined();
        await handler!({ paths: ["/Users/x/a.txt", "/Users/x/b.txt"] });

        expect(onStart).toHaveBeenCalledTimes(1);
        expect(uploadFileMock).toHaveBeenCalledTimes(2);
        const startOrder = onStart.mock.invocationCallOrder[0];
        const uploadOrder = uploadFileMock.mock.invocationCallOrder[0];
        expect(startOrder).toBeLessThan(uploadOrder);
    });

    it("does NOT call onStart for an OS-drop with empty paths", async () => {
        const onStart = vi.fn();
        render(<Harness onStart={onStart} />);
        const handler = wailsListeners.get("files:os-drop");
        await handler!({ paths: [] });
        expect(onStart).not.toHaveBeenCalled();
        expect(uploadFileMock).not.toHaveBeenCalled();
    });
});
