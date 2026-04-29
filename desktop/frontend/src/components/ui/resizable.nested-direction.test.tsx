import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "@/components/ui/resizable";

// Regression: when a horizontal ResizablePanelGroup is nested inside
// a vertical ResizablePanelGroup — the production layout, since the
// FileBrowser's files-list / files-preview split lives inside
// ProjectShell's main-content / terminal-drawer vertical split — the
// inner horizontal handle used to inherit the vertical-group
// `h-px w-full` overrides because the selectors used the descendant
// combinator (`[data-panel-group-direction=vertical] &`). The
// horizontal handle in the inner row therefore went to width:100%,
// devoured the row, and both inner panels collapsed to width=0.
// Operators saw the file explorer "go blank" the moment they
// selected a file (the click flips previewExpanded → true and the
// inner ResizablePanelGroup mounts).
//
// We can't observe layout collapse in jsdom (no flex engine), but we
// CAN verify the CSS selector contract directly: the inner handle
// must NOT match the vertical-group override selector when it sits
// under a horizontal group, regardless of any further-out vertical
// ancestor. A regression to the descendant combinator (`_`) flips
// that.

describe("nested ResizablePanelGroup direction (regression)", () => {
    it("inner horizontal handle does not match the vertical-group override", () => {
        const { container } = render(
            <div>
                <ResizablePanelGroup direction="vertical">
                    <ResizablePanel id="top" defaultSize="60%" minSize="20%">
                        <ResizablePanelGroup direction="horizontal">
                            <ResizablePanel id="L" defaultSize="62%" minSize="30%">
                                <div>L</div>
                            </ResizablePanel>
                            <ResizableHandle />
                            <ResizablePanel id="R" defaultSize="38%" minSize="20%" maxSize="70%">
                                <div>R</div>
                            </ResizablePanel>
                        </ResizablePanelGroup>
                    </ResizablePanel>
                    <ResizableHandle />
                    <ResizablePanel id="bottom" defaultSize="40%" minSize="20%">
                        <div>B</div>
                    </ResizablePanel>
                </ResizablePanelGroup>
            </div>,
        );

        const handles = Array.from(
            container.querySelectorAll<HTMLElement>("[data-slot='resizable-handle']"),
        );
        const innerHandle = handles.find(
            (h) => h.parentElement?.getAttribute("data-panel-group-direction") === "horizontal",
        );
        const outerHandle = handles.find(
            (h) => h.parentElement?.getAttribute("data-panel-group-direction") === "vertical",
        );

        expect(innerHandle, "inner handle in horizontal group").toBeDefined();
        expect(outerHandle, "outer handle in vertical group").toBeDefined();

        // Child-combinator selector — what the fix uses. The outer
        // handle is a DIRECT child of a vertical group, so it MUST
        // match. The inner handle is a direct child of a horizontal
        // group (vertical is a great-grandparent only), so it MUST
        // NOT match.
        const childSel = "[data-panel-group-direction=vertical]>*";
        expect(outerHandle!.matches(childSel)).toBe(true);
        expect(innerHandle!.matches(childSel)).toBe(false);

        // Sanity: the descendant variant (the bug shape) WOULD match
        // both, which is exactly why it broke the inner row's layout.
        const descSel = "[data-panel-group-direction=vertical] *";
        expect(outerHandle!.matches(descSel)).toBe(true);
        expect(innerHandle!.matches(descSel)).toBe(true);

        // The handle's className itself should use the child
        // combinator, not the descendant. A regression that replaces
        // `>&]:` with `_&]:` (the buggy historical shape) would flip
        // this assertion.
        expect(innerHandle!.className).toContain(
            "[[data-panel-group-direction=vertical]>&]:w-full",
        );
        expect(innerHandle!.className).not.toContain(
            "[[data-panel-group-direction=vertical]_&]:w-full",
        );
    });
});
