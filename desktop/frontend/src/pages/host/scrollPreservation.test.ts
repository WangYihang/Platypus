import { describe, expect, it } from "vitest";

import { computeScrollSwap } from "./scrollPreservation";

// computeScrollSwap is the pure brain behind HostView's per-tab
// scroll restoration. Each tab panel shares a single scroll
// container; without help, every tab change resets scrollTop to 0
// and operators lose their place when flipping between Sessions and
// Info on a long page.
//
// The helper takes the current saved-scroll map, the tab the user is
// leaving, the scrollTop it was at, and the tab they're going to,
// and returns:
//   · the next saved-scroll map (with the leaving tab's offset stored)
//   · the scrollTop value to apply to the container for the new tab
// Pure data so we can test the behaviour without touching the DOM.

describe("computeScrollSwap", () => {
    it("saves the leaving tab's offset and restores 0 for an unseen tab", () => {
        const result = computeScrollSwap(
            new Map(),
            "info",
            240,
            "sessions",
        );
        expect(result.scrollTop).toBe(0);
        expect(result.map.get("info")).toBe(240);
    });

    it("restores a previously-saved offset on return", () => {
        const result = computeScrollSwap(
            new Map([["info", 240]]),
            "sessions",
            120,
            "info",
        );
        expect(result.scrollTop).toBe(240);
        expect(result.map.get("sessions")).toBe(120);
    });

    it("returns a new map instance (no mutation)", () => {
        const before = new Map([["info", 100]]);
        const result = computeScrollSwap(before, "info", 999, "files");
        expect(result.map).not.toBe(before);
        expect(before.get("info")).toBe(100);
    });

    it("treats null leaving tab as a no-op for the saved-scroll side", () => {
        // First mount: no prior tab, just go to the requested tab.
        const result = computeScrollSwap(new Map(), null, 0, "info");
        expect(result.scrollTop).toBe(0);
        expect(result.map.size).toBe(0);
    });
});
