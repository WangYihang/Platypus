import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// F6 part 2: ProjectsLanding used to show the shell-level Loader2
// spinner during the initial /projects fetch, then hard-swap to the
// populated tile grid. The transition flashed a centred spinner and
// then the layout snapped into place — looked like the page was
// thrashing on every navigate / refresh, especially on Login →
// /projects.
//
// Replace the spinner with a tile-grid skeleton matching the
// populated layout's column shape, so loaded ↔ loading is a content
// swap instead of a layout pop. Each placeholder tile is tagged
// data-testid="project-tile-skeleton" so the spec doesn't depend
// on Tailwind / Skeleton class names.
test.describe("ProjectsLanding skeleton", () => {
    test("renders tile placeholders during the projects fetch", async ({ page }) => {
        await loginAsAdmin(page);

        // Slow the projects API enough that we can observe the
        // skeleton state. listProjects is the only thing the
        // landing page waits on.
        await page.route(/\/api\/v1\/projects(?:\?|$)/, async (route) => {
            await new Promise((r) => setTimeout(r, 1500));
            return route.continue();
        });

        await page.goto("/projects");

        const tiles = page.locator('[data-testid="project-tile-skeleton"]');
        await expect(
            tiles.first(),
            "skeleton placeholder didn't render during the projects fetch",
        ).toBeVisible({ timeout: 5_000 });
        const count = await tiles.count();
        expect(
            count,
            `expected at least 3 skeleton tiles; got ${count}`,
        ).toBeGreaterThanOrEqual(3);

        // Once the response lands, the skeletons must go away — no
        // permanent layout-shifting double render.
        await expect(tiles.first()).toBeHidden({ timeout: 10_000 });
    });
});
