import { describe, expect, it } from "vitest";

import { memberRemovalWarning } from "./warnings";

// memberRemovalWarning produces a one-line consequence string the
// remove-member confirmation dialog surfaces above the standard copy.
// The contract:
//
//   · Removing the last member of a project is the most jarring case
//     because the project ends up with zero explicit members. Surface
//     a clear "This is the last member" warning then.
//
//   · Removing a project-admin (when others remain) is also worth
//     calling out — losing the project's admin is recoverable but
//     surprising.
//
//   · For ordinary removals (operator/viewer with siblings around),
//     no warning — the standard description already covers it.

describe("memberRemovalWarning", () => {
    it("warns when removing the last member", () => {
        const out = memberRemovalWarning({ memberCount: 1, isProjectAdmin: false });
        expect(out).toMatch(/last member/i);
    });

    it("warns when removing a project admin (with others remaining)", () => {
        const out = memberRemovalWarning({ memberCount: 4, isProjectAdmin: true });
        expect(out).toMatch(/admin/i);
    });

    it("returns null for an ordinary operator/viewer removal", () => {
        const out = memberRemovalWarning({ memberCount: 5, isProjectAdmin: false });
        expect(out).toBeNull();
    });

    it("prefers the last-member warning over the admin warning", () => {
        // memberCount of 1 means "this is the only member"; even if
        // they're an admin, the louder fact is that the project will
        // have no members at all.
        const out = memberRemovalWarning({ memberCount: 1, isProjectAdmin: true });
        expect(out).toMatch(/last member/i);
    });
});
