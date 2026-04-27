import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// P5: lib/icons.ts is the closed registry that maps a domain noun
// (project / fleet / host / session / etc.) to the lucide icon that
// represents it across the entire SPA. The point is to avoid the
// muscle-memory drift where the same noun ended up with three
// different glyphs across pages.
//
// We don't pin specific icon component names (lucide can rename
// internally), but we DO require:
//
//   1. The registry exports the well-known nouns the sidebar uses.
//   2. The sidebar renders SVG glyphs for each visible nav item, i.e.
//      it actually consumed the registry rather than hand-rolling
//      each glyph.
//   3. Each nav item has exactly one icon (no double-icon drift while
//      a refactor is in progress).
test.describe("icon registry adoption", () => {
    test("registry exports the well-known nouns", async ({ page }) => {
        await page.goto("/");
        const keys = await page.evaluate(async () => {
            const m = await import("/src/lib/icons.ts");
            return Object.keys(m.icons).sort();
        });
        for (const required of [
            "project",
            "fleet",
            "activity",
            "enrollment",
            "members",
            "settings",
            "host",
            "session",
            "accessToken",
            "installCommand",
            "mesh",
        ]) {
            expect(keys, `lib/icons.ts is missing '${required}'`).toContain(required);
        }
    });

    test("sidebar nav items each render exactly one icon", async ({ page }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/overview");

        // The sidebar nav lives inside ProjectSidebar's <nav>. Each
        // active or inactive NavLink renders text + an icon span.
        const navLinks = page.locator(".pl-nav-link");
        const count = await navLinks.count();
        expect(count, "expected at least 4 nav links").toBeGreaterThanOrEqual(4);

        for (let i = 0; i < count; i++) {
            const link = navLinks.nth(i);
            const svgCount = await link.locator("svg").count();
            const label = (await link.textContent())?.trim() ?? "";
            expect(
                svgCount,
                `nav link "${label}" rendered ${svgCount} svgs; expected exactly 1`,
            ).toBe(1);
        }
    });
});
