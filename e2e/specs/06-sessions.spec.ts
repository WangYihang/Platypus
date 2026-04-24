import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("sessions", () => {
    test("Sessions page renders with Live / All filter chips", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Sessions$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/sessions$/);

        // Filter chips always render regardless of rows.
        await expect(page.getByText("Live", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("All", { exact: true })).toBeVisible();

        // v2 doesn't auto-create session rows on agent connect (only
        // host rows), so we only assert the page chrome. Specs that
        // need live session data must drive it through the host
        // Terminal flow.
        await page.screenshot({
            path: shotPath("09-sessions-list.png"),
            fullPage: false,
        });
    });
});
