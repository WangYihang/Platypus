import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// F6 part 3: ProjectOverview rendered the populated KPI cards
// immediately on mount with `0` / `—` placeholders, then snapped to
// real numbers when the parallel listHosts + listProjectSessions
// fetches resolved. The "0" digits read as content, so users
// momentarily saw a dashboard claiming "0 hosts, 0 sessions" before
// the real values appeared — easy to misread as "the project is
// empty".
//
// Replace the brief misleading-zeros window with a skeleton state:
// KPI tile placeholders + a chart placeholder + a feed placeholder
// while the data is in flight. Each placeholder is tagged
// data-testid="overview-skeleton" so the spec can detect it without
// coupling to Tailwind class names.
test.describe("ProjectOverview skeleton", () => {
    test("placeholder appears during the hosts + sessions fetch", async ({ page }) => {
        await loginAsAdmin(page);

        // Slow BOTH endpoints the page waits on so the skeleton stays
        // up long enough to observe.
        await page.route(/\/api\/v1\/projects\/[^/]+\/hosts(?:\?|$)/, async (route) => {
            await new Promise((r) => setTimeout(r, 1500));
            return route.continue();
        });
        await page.route(
            /\/api\/v1\/projects\/[^/]+\/sessions(?:\?|$)/,
            async (route) => {
                await new Promise((r) => setTimeout(r, 1500));
                return route.continue();
            },
        );

        await page.goto("/projects/default/overview");

        const skel = page.locator('[data-testid="overview-skeleton"]');
        await expect(
            skel.first(),
            "overview skeleton placeholder didn't render during fetch",
        ).toBeVisible({ timeout: 5_000 });
        const count = await skel.count();
        expect(count, `expected at least 4 skeleton blocks; got ${count}`).toBeGreaterThanOrEqual(
            4,
        );

        // Skeletons go away once data lands.
        await expect(skel.first()).toBeHidden({ timeout: 10_000 });
    });
});
