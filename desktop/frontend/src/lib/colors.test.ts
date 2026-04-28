import { describe, expect, it } from "vitest";

import { palette } from "../layout/theme";
import { colorForId, contrastingTextColor } from "./colors";

// colorForId is the deterministic "this id always picks the same
// colour" helper. Used by ServerRail (already, manually) and now by
// the terminal drawer + status-bar terminals popover so each host
// gets a stable, distinguishable visual identity. Backed by the
// curated palette.avatarBgs so the colours match what operators
// already see on the server rail.

describe("colorForId", () => {
    it("returns a colour from palette.avatarBgs", () => {
        expect(palette.avatarBgs).toContain(colorForId("a"));
        expect(palette.avatarBgs).toContain(colorForId("a-very-long-machine-id"));
    });

    it("is stable for the same input", () => {
        const c = colorForId("machine-1");
        expect(colorForId("machine-1")).toBe(c);
        expect(colorForId("machine-1")).toBe(c);
    });

    it("falls back to a default when the id is empty", () => {
        // Empty / nullish ids should still return a usable colour
        // (e.g. for hosts whose machine_id hasn't been reported yet),
        // not crash and not return undefined.
        expect(palette.avatarBgs).toContain(colorForId(""));
        expect(palette.avatarBgs).toContain(colorForId(null));
        expect(palette.avatarBgs).toContain(colorForId(undefined));
    });

    it("distributes ids across more than one bucket", () => {
        // Hash collisions are inevitable but we shouldn't be funnelling
        // every id into a single colour. Ten buckets, 100 ids → expect
        // at least three distinct colours.
        const seen = new Set<string>();
        for (let i = 0; i < 100; i++) {
            seen.add(colorForId(`id-${i}`));
        }
        expect(seen.size).toBeGreaterThanOrEqual(3);
    });
});

describe("contrastingTextColor", () => {
    it("picks white for dark backgrounds", () => {
        // #3b82f6 (blue) and #6366f1 (indigo) are dark enough that
        // white text is readable on them.
        expect(contrastingTextColor("#3b82f6")).toBe("#ffffff");
        expect(contrastingTextColor("#000000")).toBe("#ffffff");
    });

    it("picks black for light backgrounds", () => {
        expect(contrastingTextColor("#ffffff")).toBe("#000000");
        expect(contrastingTextColor("#fafafa")).toBe("#000000");
    });
});
