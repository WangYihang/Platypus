import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { FileEntryDTO } from "@wails/go/app/App";
import FileContextMenu from "./FileContextMenu";

function entry(overrides: Partial<FileEntryDTO> = {}): FileEntryDTO {
    return {
        name: "file.txt",
        size: 100,
        mode: 0o644,
        modTimeUnix: 0,
        isDir: false,
        isSymlink: false,
        ...overrides,
    };
}

function openMenu(testId = "row") {
    fireEvent.contextMenu(screen.getByTestId(testId));
}

describe("<FileContextMenu> (row variant, single entry)", () => {
    it("dispatches Open, Download, Rename, Chmod, Copy path/name, Delete callbacks", () => {
        const onOpen = vi.fn();
        const onDownload = vi.fn();
        const onRename = vi.fn();
        const onChmod = vi.fn();
        const onCopyPath = vi.fn();
        const onCopyName = vi.fn();
        const onDelete = vi.fn();

        render(
            <FileContextMenu
                variant={{ kind: "row", entries: [entry({ name: "doc.txt" })] }}
                onOpen={onOpen}
                onDownload={onDownload}
                onRename={onRename}
                onChmod={onChmod}
                onCopyPath={onCopyPath}
                onCopyName={onCopyName}
                onDelete={onDelete}
            >
                <div data-testid="row">doc.txt</div>
            </FileContextMenu>,
        );

        openMenu();
        fireEvent.click(screen.getByText(/^preview$/i));
        expect(onOpen).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/^download$/i));
        expect(onDownload).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/^rename$/i));
        expect(onRename).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/^chmod$/i));
        expect(onChmod).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/copy path/i));
        expect(onCopyPath).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/copy name/i));
        expect(onCopyName).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/^delete$/i));
        expect(onDelete).toHaveBeenCalled();
    });

    it("labels Open as 'Open' for folders and 'Preview' for files", () => {
        const { rerender } = render(
            <FileContextMenu
                variant={{ kind: "row", entries: [entry({ isDir: true, name: "src" })] }}
                onOpen={() => {}}
            >
                <div data-testid="row">src</div>
            </FileContextMenu>,
        );
        openMenu();
        expect(screen.getByText(/^open$/i)).toBeInTheDocument();

        rerender(
            <FileContextMenu
                variant={{ kind: "row", entries: [entry({ isDir: false })] }}
                onOpen={() => {}}
            >
                <div data-testid="row">file</div>
            </FileContextMenu>,
        );
        // Re-open after the rerender.
        openMenu();
        expect(screen.getByText(/^preview$/i)).toBeInTheDocument();
    });
});

describe("<FileContextMenu> (row variant, multi-select)", () => {
    it("hides Rename and Chmod when more than one entry is selected", () => {
        const entries = [entry({ name: "a" }), entry({ name: "b" })];
        render(
            <FileContextMenu
                variant={{ kind: "row", entries }}
                onRename={() => {}}
                onChmod={() => {}}
                onDelete={() => {}}
            >
                <div data-testid="row">2 selected</div>
            </FileContextMenu>,
        );
        openMenu();
        expect(screen.queryByText(/^rename$/i)).toBeNull();
        expect(screen.queryByText(/^chmod$/i)).toBeNull();
        // Multi-friendly actions still visible.
        expect(screen.getByText(/^delete$/i)).toBeInTheDocument();
    });
});

describe("<FileContextMenu> (empty variant)", () => {
    it("renders New file / New folder / Upload here / Refresh and dispatches callbacks", () => {
        const onNewFile = vi.fn();
        const onNewFolder = vi.fn();
        const onUploadHere = vi.fn();
        const onRefresh = vi.fn();

        render(
            <FileContextMenu
                variant={{ kind: "empty" }}
                onNewFile={onNewFile}
                onNewFolder={onNewFolder}
                onUploadHere={onUploadHere}
                onRefresh={onRefresh}
            >
                <div data-testid="row">empty pane</div>
            </FileContextMenu>,
        );

        openMenu();
        fireEvent.click(screen.getByText(/new file/i));
        expect(onNewFile).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/new folder/i));
        expect(onNewFolder).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/upload here/i));
        expect(onUploadHere).toHaveBeenCalled();

        openMenu();
        fireEvent.click(screen.getByText(/^refresh$/i));
        expect(onRefresh).toHaveBeenCalled();
    });

    it("includes a disabled Paste placeholder", () => {
        // Clipboard paste isn't wired up yet (separate feature). The
        // item exists so the menu mirrors OS conventions, but it's
        // disabled until clipboard work lands so users don't trigger
        // a no-op.
        render(
            <FileContextMenu variant={{ kind: "empty" }}>
                <div data-testid="row">empty pane</div>
            </FileContextMenu>,
        );
        openMenu();
        const paste = screen.getByText(/^paste$/i).closest("[data-slot='context-menu-item']");
        expect(paste).not.toBeNull();
        expect(paste?.getAttribute("data-disabled")).not.toBeNull();
    });
});
