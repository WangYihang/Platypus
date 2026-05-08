import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Sessions used to be the Fleet's Timeline view; the Fleet → Hosts IA
// move promoted the project-wide rollups (Sessions / Events /
// Recordings / Transfers) into their own /activity/<sub-tab>
// surface. The Live / All filter chips moved with it.
test.describe("activity · sessions (live timeline)", () => {
    test("Sessions tab renders with Live / All filter chips", async ({ page }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/activity/sessions");
        await expect(page).toHaveURL(/\/projects\/default\/activity\/sessions$/);

        // Filter chips always render regardless of rows.
        await expect(page.getByText("Live", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("All", { exact: true })).toBeVisible();

        await page.screenshot({
            path: shotPath("09-sessions-list.png"),
            fullPage: false,
        });
    });
});
