import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DndContext } from "@dnd-kit/core";

import FileGrid from "./FileGrid";
import type { FileEntryDTO } from "../../../platform/App.web";

const dir = (name: string): FileEntryDTO => ({
    name,
    size: 0,
    mode: 0o755,
    modTimeUnix: 0,
    isDir: true,
    isSymlink: false,
});

const file = (
    name: string,
    extra: Partial<FileEntryDTO> = {},
): FileEntryDTO => ({
    name,
    size: 1024,
    mode: 0o644,
    modTimeUnix: 0,
    isDir: false,
    isSymlink: false,
    ...extra,
});

function renderGrid(props: Partial<React.ComponentProps<typeof FileGrid>> = {}) {
    const onOpen = vi.fn();
    const setSelectedNames = vi.fn();
    const utils = render(
        <DndContext>
            <FileGrid
                entries={props.entries ?? []}
                currentPath="/x"
                selectedNames={props.selectedNames ?? new Set()}
                setSelectedNames={setSelectedNames}
                onOpen={onOpen}
            />
        </DndContext>,
    );
    return { ...utils, onOpen, setSelectedNames };
}

describe("<FileGrid>", () => {
    it("shows an empty state when there are no entries", () => {
        renderGrid();
        expect(screen.getByText(/empty directory/i)).toBeInTheDocument();
    });

    it("renders one tile per entry with the file name", () => {
        renderGrid({ entries: [dir("src"), file("README.md"), file("logo.png", { mime: "image/png" })] });
        expect(screen.getByText("src")).toBeInTheDocument();
        expect(screen.getByText("README.md")).toBeInTheDocument();
        expect(screen.getByText("logo.png")).toBeInTheDocument();
    });

    it("invokes onOpen when a tile is double-clicked", () => {
        const entries = [file("notes.txt")];
        const { onOpen } = renderGrid({ entries });
        const tile = screen.getByRole("button", { name: /notes\.txt/ });
        fireEvent.doubleClick(tile);
        expect(onOpen).toHaveBeenCalledWith(entries[0]);
    });

    it("toggles selection on single click", () => {
        const entries = [file("a.txt"), file("b.txt")];
        const { setSelectedNames } = renderGrid({ entries });
        fireEvent.click(screen.getByRole("button", { name: /a\.txt/ }));
        expect(setSelectedNames).toHaveBeenCalled();
        const arg = setSelectedNames.mock.calls[0][0] as Set<string>;
        expect(arg.has("a.txt")).toBe(true);
    });
});
