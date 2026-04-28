import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import FileDropZone from "./FileDropZone";

// Helper: build a synthetic DataTransfer-like object with files. The
// real DataTransfer constructor exists in jsdom but its files property
// is read-only; tests fake it with a plain object that exposes the
// minimal surface FileDropZone reads.
function dragEventInit(files: File[]): { dataTransfer: { files: FileList; types: string[]; items?: unknown } } {
    return {
        dataTransfer: {
            files: makeFileList(files),
            types: ["Files"],
        },
    };
}

function makeFileList(files: File[]): FileList {
    // FileList isn't constructible in jsdom; simulate with an array
    // that exposes .item() + length + indexed access.
    return Object.assign([...files], {
        item(i: number) {
            return files[i] ?? null;
        },
        length: files.length,
    }) as unknown as FileList;
}

function makeFile(name: string, bytes: number): File {
    return new File([new Uint8Array(bytes)], name, { type: "application/octet-stream" });
}

describe("FileDropZone", () => {
    it("renders its children when no drag is active", () => {
        render(
            <FileDropZone onDrop={() => {}}>
                <div data-testid="content">browser content</div>
            </FileDropZone>,
        );
        expect(screen.getByTestId("content")).toBeInTheDocument();
        // Overlay is hidden initially.
        expect(screen.queryByText(/drop files/i)).toBeNull();
    });

    it("shows a 'drop files here' overlay on drag-enter and hides on drag-leave", () => {
        render(
            <FileDropZone onDrop={() => {}}>
                <div data-testid="content">x</div>
            </FileDropZone>,
        );
        const root = screen.getByTestId("content").parentElement!;
        fireEvent.dragEnter(root, dragEventInit([]));
        expect(screen.getByText(/drop files/i)).toBeInTheDocument();
        fireEvent.dragLeave(root, dragEventInit([]));
        expect(screen.queryByText(/drop files/i)).toBeNull();
    });

    it("invokes onDrop with the dropped files", () => {
        const onDrop = vi.fn();
        render(
            <FileDropZone onDrop={onDrop}>
                <div data-testid="content">x</div>
            </FileDropZone>,
        );
        const root = screen.getByTestId("content").parentElement!;
        const a = makeFile("a.bin", 32);
        const b = makeFile("b.bin", 64);
        fireEvent.dragEnter(root, dragEventInit([a, b]));
        fireEvent.drop(root, dragEventInit([a, b]));
        expect(onDrop).toHaveBeenCalledTimes(1);
        const arg = onDrop.mock.calls[0][0] as File[];
        expect(arg).toHaveLength(2);
        expect(arg[0].name).toBe("a.bin");
        expect(arg[1].name).toBe("b.bin");
    });

    it("does NOT fire onDrop when the drag wasn't a file drag", () => {
        const onDrop = vi.fn();
        render(
            <FileDropZone onDrop={onDrop}>
                <div data-testid="content">x</div>
            </FileDropZone>,
        );
        const root = screen.getByTestId("content").parentElement!;
        // Simulate a text-only drag (e.g. selected text being dragged).
        fireEvent.drop(root, {
            dataTransfer: {
                files: makeFileList([]),
                types: ["text/plain"],
            },
        });
        expect(onDrop).not.toHaveBeenCalled();
    });

    it("hides the overlay after a successful drop", () => {
        render(
            <FileDropZone onDrop={() => {}}>
                <div data-testid="content">x</div>
            </FileDropZone>,
        );
        const root = screen.getByTestId("content").parentElement!;
        const f = makeFile("a.bin", 1);
        fireEvent.dragEnter(root, dragEventInit([f]));
        expect(screen.getByText(/drop files/i)).toBeInTheDocument();
        fireEvent.drop(root, dragEventInit([f]));
        expect(screen.queryByText(/drop files/i)).toBeNull();
    });

    it("respects a `disabled` prop by ignoring drops", () => {
        const onDrop = vi.fn();
        render(
            <FileDropZone onDrop={onDrop} disabled>
                <div data-testid="content">x</div>
            </FileDropZone>,
        );
        const root = screen.getByTestId("content").parentElement!;
        const f = makeFile("a.bin", 1);
        fireEvent.dragEnter(root, dragEventInit([f]));
        // Overlay shouldn't appear when disabled.
        expect(screen.queryByText(/drop files/i)).toBeNull();
        fireEvent.drop(root, dragEventInit([f]));
        expect(onDrop).not.toHaveBeenCalled();
    });
});
