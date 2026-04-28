import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
    ContextMenu,
    ContextMenuContent,
    ContextMenuItem,
    ContextMenuSeparator,
    ContextMenuTrigger,
} from "./context-menu";

describe("<ContextMenu>", () => {
    it("opens on a right-click and shows its items", () => {
        render(
            <ContextMenu>
                <ContextMenuTrigger>
                    <div data-testid="row">target</div>
                </ContextMenuTrigger>
                <ContextMenuContent>
                    <ContextMenuItem>Open</ContextMenuItem>
                    <ContextMenuSeparator />
                    <ContextMenuItem>Delete</ContextMenuItem>
                </ContextMenuContent>
            </ContextMenu>,
        );

        // Items aren't rendered until the menu opens.
        expect(screen.queryByText("Open")).toBeNull();

        fireEvent.contextMenu(screen.getByTestId("row"));

        expect(screen.getByText("Open")).toBeInTheDocument();
        expect(screen.getByText("Delete")).toBeInTheDocument();
    });

    it("invokes onSelect when an item is clicked", () => {
        let opened = false;
        render(
            <ContextMenu>
                <ContextMenuTrigger>
                    <div data-testid="row">target</div>
                </ContextMenuTrigger>
                <ContextMenuContent>
                    <ContextMenuItem onSelect={() => (opened = true)}>Open</ContextMenuItem>
                </ContextMenuContent>
            </ContextMenu>,
        );

        fireEvent.contextMenu(screen.getByTestId("row"));
        fireEvent.click(screen.getByText("Open"));

        expect(opened).toBe(true);
    });
});
