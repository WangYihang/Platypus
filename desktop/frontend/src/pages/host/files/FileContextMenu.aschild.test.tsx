import * as React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { FileEntryDTO } from "@wails/go/app/App";
import FileContextMenu from "./FileContextMenu";

// Regression spec: when a per-row FileContextMenu wraps a CUSTOM
// component (Tile in FileGrid, DraggableRow in FileTable) instead of
// a raw DOM element, Radix's `<ContextMenuTrigger asChild>` clones the
// child and tries to attach an `onContextMenu` listener via a forwarded
// ref. If the wrapped component doesn't `forwardRef` and doesn't
// spread incoming props onto the underlying element, the listener
// silently doesn't reach the DOM — right-clicks bubble straight past
// the row trigger to whatever outer trigger the page has, and the user
// sees the wrong menu.
//
// This bug actually shipped because both Tile and DraggableRow were
// declared as plain `function Tile(props) { ... }` components. Both
// have since been converted to `React.forwardRef` with a `...rest`
// spread; the test below pins the asChild contract by wrapping a
// minimal forwardRef component that mirrors that pattern.

const Tile = React.forwardRef<HTMLButtonElement, React.ButtonHTMLAttributes<HTMLButtonElement>>(
    function Tile(props, ref) {
        return (
            <button ref={ref} data-testid="tile" {...props}>
                tile
            </button>
        );
    },
);

function entry(): FileEntryDTO {
    return {
        name: "doc.txt",
        size: 100,
        mode: 0o644,
        modTimeUnix: 0,
        isDir: false,
        isSymlink: false,
    };
}

describe("<FileContextMenu> asChild forwarding", () => {
    it("opens the row menu when right-clicking a forwardRef child", () => {
        const onDownload = vi.fn();
        render(
            <FileContextMenu
                variant={{ kind: "row", entries: [entry()] }}
                onDownload={onDownload}
                onRename={vi.fn()}
                onDelete={vi.fn()}
            >
                <Tile />
            </FileContextMenu>,
        );

        // Right-click on the wrapped Tile. If the inner trigger isn't
        // wired (the bug), Download wouldn't be in the document at
        // all — the row menu never opens.
        fireEvent.contextMenu(screen.getByTestId("tile"));
        expect(screen.getByText(/^download$/i)).toBeInTheDocument();
    });
});
