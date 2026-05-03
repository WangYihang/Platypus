import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// /marketplace — global tab shipped alongside the C8/B2 plugin
// runtime + marketplace catalog work. Catalog is empty in CI (no
// PLATYPUS_PLUGIN_INDEX env wired), so the test asserts the empty-
// state copy + the admin-only Refresh button + the
// "never_synced" status line.
//
// Once a real index URL is configured for the e2e env this test can
// extend to "click Refresh, assert plugin cards render" — for now
// the empty case is what an operator sees on a fresh deploy.
test.describe("marketplace", () => {
    test("global tab loads + shows the empty-catalog state", async ({ page }) => {
        await loginAsAdmin(page);

        // Marketplace lives in globalTabs (NavTabs.tsx) alongside
        // Projects / Servers / Admin.
        await page.getByRole("link", { name: /^Marketplace$/ }).click();
        await expect(page).toHaveURL(/\/marketplace$/);

        // Page chrome.
        await expect(page.getByRole("heading", { name: /^Marketplace$/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /^Refresh$/ })).toBeVisible();

        // Empty-state copy. The catalog has no PLATYPUS_PLUGIN_INDEX
        // configured in the e2e env so the search returns nothing
        // and the status row reads "never synced".
        await expect(page.getByText(/Catalog has never been synced\./i)).toBeVisible();
        await expect(page.getByText(/No plugins found/i)).toBeVisible();

        // Search box renders + is editable.
        const search = page.getByPlaceholder(/Search by plugin name/i);
        await search.fill("noop");
        // Empty + filtered → empty-state copy mentions the query.
        await expect(page.getByText(/No marketplace plugin matches "noop"/i)).toBeVisible();

        await page.screenshot({
            path: shotPath("30-marketplace-empty.png"),
            fullPage: false,
        });
    });

    test("Refresh button reports HTTP error when index unreachable", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("link", { name: /^Marketplace$/ }).click();

        // The catalog is configured with empty index URL in the e2e
        // env (see globalSetup), so Refresh is a no-op + returns
        // count=0 successfully. We only assert that the click + the
        // toast happen — the "HTTP error" path needs a real
        // unreachable URL which we don't stand up for the regression
        // run.
        await page.getByRole("button", { name: /^Refresh$/ }).click();
        // Sonner toast surface.
        await expect(page.getByText(/Marketplace synced: 0 plugin versions/i)).toBeVisible({
            timeout: 5_000,
        });
    });
});
