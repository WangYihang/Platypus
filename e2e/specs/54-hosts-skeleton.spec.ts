import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// F6: list pages used to render a centred Loader2 spinner while the
// data loaded, then jump to the populated table on arrival. The
// layout shift made the page look like it was thrashing on every
// navigate — especially on ProjectsLanding, FleetPage's HostsPanel,
// and ProjectOverview where the populated layout is large and
// distinctive.
//
// HostsPanel (the most-visited list surface) gets a skeleton table
// matching the real shape — same column widths, same header — so the
// transition into real data is a content swap rather than a layout
// pop. We test by intercepting the hosts API to slow the response,
// then assert that during the delay the table skeleton's pulse rows
// are visible at the right shape.
test.describe("HostsPanel — skeleton table while loading", () => {
    test("shows N skeleton rows during the hosts fetch, then real data", async ({
        page,
    }) => {
        await loginAsAdmin(page);

        // Slow the hosts API so we can observe the skeleton.
        await page.route(/\/api\/v1\/projects\/[^/]+\/hosts(?:\?|$)/, async (route) => {
            await new Promise((r) => setTimeout(r, 1500));
            return route.continue();
        });

        await page.goto("/projects/default/hosts");

        // The skeleton should render at least 4 placeholder rows. We
        // mark each with data-testid="hosts-row-skeleton" so the test
        // doesn't depend on the exact lucide / pulse class plumbing.
        const skeletons = page.locator('[data-testid="hosts-row-skeleton"]');
        await expect(
            skeletons.first(),
            "hosts skeleton placeholder didn't render during fetch",
        ).toBeVisible({ timeout: 5_000 });
        const count = await skeletons.count();
        expect(count, `expected ≥4 skeleton rows; got ${count}`).toBeGreaterThanOrEqual(4);

        // Once the response lands, the skeletons must go away — no
        // permanent layout-shifting double render.
        await expect(skeletons.first()).toBeHidden({ timeout: 10_000 });
    });
});
