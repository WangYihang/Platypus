import { describe, expect, it } from "vitest";

import { applyQuickFilter } from "./quickFilters";

// applyQuickFilter is the pure brain behind the Activities page's
// quick-filter chips. The chips don't keep their own state — they
// emit a partial filter patch (range / actor / outcome / categories
// / query) that the page merges with its existing filter state.
//
// The chips matter because the existing toolbar requires 3-4 control
// changes for common queries. "My actions" is one click instead of
// "set range = 24h" + "type my username into actor".

describe("applyQuickFilter", () => {
    it("'my' scopes to the signed-in user over the last 24h", () => {
        expect(applyQuickFilter("my", { username: "ada" })).toEqual({
            actor: "ada",
            range: "24h",
        });
    });

    it("'failures' filters to error outcomes", () => {
        expect(applyQuickFilter("failures", { username: "ada" })).toEqual({
            outcome: "error",
        });
    });

    it("'24h' narrows the time window without touching other filters", () => {
        expect(applyQuickFilter("24h", { username: "ada" })).toEqual({
            range: "24h",
        });
    });

    it("'clear' resets every filter to its default", () => {
        expect(applyQuickFilter("clear", { username: "ada" })).toEqual({
            actor: "",
            outcome: "",
            query: "",
            range: "7d",
            categories: [],
        });
    });
});
