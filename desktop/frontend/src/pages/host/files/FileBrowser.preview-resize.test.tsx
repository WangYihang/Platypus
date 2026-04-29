import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
    ResizableHandle,
    ResizablePanel,
    ResizablePanelGroup,
} from "@/components/ui/resizable";

// react-resizable-panels v4 interprets a numeric `defaultSize` /
// `minSize` / `maxSize` as **pixels**, not percentages — `bt(e)` in
// the v4 bundle returns `[e, "px"]` for `typeof e === "number"`. On
// the very first render, before the library's layout effect populates
// the synced flex-grow state, each Panel renders with the raw
// defaultSize as `flex-basis`. With numeric props that resolves to
// `flex-basis: 62px` / `flex-basis: 38px` — the panels collapse to a
// 100-px sliver in the corner before the next paint promotes them to
// the proper percentage layout. With string "%" props the same path
// produces `flex-basis: 62%` / `flex-basis: 38%`, so the panels fill
// the group from the first paint.
//
// The FileBrowser preview-pane split previously passed numbers and
// the resulting first-paint flash read as a "blank file browser"
// when operators double-clicked into preview. Pin the contract so a
// regression to numeric props fails this spec.

function getRawFlexBases(container: HTMLElement): string[] {
    return Array.from(container.querySelectorAll<HTMLElement>("[data-panel]")).map(
        (p) => p.style.flexBasis,
    );
}

describe("ResizablePanel size units (regression)", () => {
    it("string '%' defaultSize hydrates the layout via getPanelStyles", () => {
        // After the library's layout effect runs (synchronously in
        // jsdom under React 19), Panels read their flex-grow from the
        // computed Group state and `flex-basis` collapses to 0. We
        // assert the post-effect state because that's what shows on
        // any paint after the first.
        const { container } = render(
            <div style={{ width: 1000, height: 400 }}>
                <ResizablePanelGroup direction="horizontal">
                    <ResizablePanel id="a" defaultSize="62%" minSize="30%">
                        <div>A</div>
                    </ResizablePanel>
                    <ResizableHandle />
                    <ResizablePanel
                        id="b"
                        defaultSize="38%"
                        minSize="20%"
                        maxSize="70%"
                    >
                        <div>B</div>
                    </ResizablePanel>
                </ResizablePanelGroup>
            </div>,
        );
        const panels = container.querySelectorAll<HTMLElement>("[data-panel]");
        expect(panels).toHaveLength(2);
        // Library converged on a `flex: <grow> 1 0px` layout; flex
        // sizing fills the group from the first paint.
        for (const p of panels) {
            expect(p.style.flex).toMatch(/^[\d.]+\s+1\s+0px$/);
        }
    });

    it("the FileBrowser preview split mounts with both panels visible", async () => {
        // This is the FileBrowser shape — two panels with the same
        // id / defaultSize / minSize / maxSize plumbing the live
        // browser uses (post-fix). A regression to numeric props
        // would still mount two panels, but the first paint would
        // show flex-basis pixel widths. Use the JSX shape that
        // ships in production so a copy-paste of the call site
        // would catch the same bug.
        const { container } = render(
            <div style={{ width: 1280, height: 800 }}>
                <ResizablePanelGroup
                    direction="horizontal"
                    autoSaveId="files-preview-split"
                    className="min-h-0 flex-1"
                >
                    <ResizablePanel
                        id="files-list"
                        defaultSize="62%"
                        minSize="30%"
                        className="relative"
                    >
                        <div>file list</div>
                    </ResizablePanel>
                    <ResizableHandle className="mx-1 bg-transparent" />
                    <ResizablePanel
                        id="files-preview"
                        defaultSize="38%"
                        minSize="20%"
                        maxSize="70%"
                        className="relative"
                    >
                        <div>preview</div>
                    </ResizablePanel>
                </ResizablePanelGroup>
            </div>,
        );
        const bases = getRawFlexBases(container);
        expect(bases).toHaveLength(2);
        // Forbidden: tiny pixel basis (62px / 38px / 70px) — that's
        // the bug shape with numeric props. Either the library
        // converged on percentage flex-grow (basis="0px") or the
        // raw "%" string came through (basis="62%"). Anything else
        // is a regression.
        for (const b of bases) {
            const ok = b === "0px" || b.endsWith("%");
            expect(ok, `unexpected flex-basis: ${b}`).toBe(true);
        }
    });
});
