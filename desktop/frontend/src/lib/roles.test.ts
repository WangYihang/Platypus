import { describe, expect, it } from "vitest";

import { ROLE_DESCRIPTION, ROLES, formatRoleSummary } from "./roles";

// roles.ts is the single source of truth for what each user role
// means in plain language. ProjectMembers / AdminUsers / UserMenu all
// surface "operator", "viewer", "admin" labels — without explaining
// what each role can do, the labels are jargon. This module backs a
// tooltip on every column header that displays a role.

describe("roles", () => {
    it("describes every role the app recognises", () => {
        for (const r of ROLES) {
            expect(ROLE_DESCRIPTION[r].length).toBeGreaterThan(0);
        }
    });

    it("orders roles from most to least privileged", () => {
        // The order matters for tooltip rendering — admin first reads
        // top-down as "decreasing privilege" instead of alphabetical.
        expect(ROLES).toEqual(["admin", "operator", "viewer"]);
    });

    it("formatRoleSummary joins each role and its description", () => {
        const out = formatRoleSummary();
        expect(out).toMatch(/admin/i);
        expect(out).toMatch(/operator/i);
        expect(out).toMatch(/viewer/i);
        // The full summary should be self-contained — no need to
        // hover individual labels.
        expect(out.length).toBeGreaterThan(80);
    });
});
