import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("sessions", () => {
    test("Live filter shows the baseline agent's session", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Sessions$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/sessions$/);

        // Segmented filter chips
        await expect(page.getByText("Live", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("All", { exact: true })).toBeVisible();

        // baseline agent → at least one row, status "live"
        const rows = page.locator("table tbody tr");
        await expect(rows.first()).toBeVisible({ timeout: 10_000 });
        await expect(page.getByText("live", { exact: true }).first()).toBeVisible();

        await page.screenshot({
            path: shotPath("09-sessions-list.png"),
            fullPage: false,
        });
    });
});
