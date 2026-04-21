import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("sessions", () => {
    test("Live/All filter + empty state", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Sessions$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/sessions$/);

        // Segmented filter chips
        await expect(page.getByText("Live", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("All", { exact: true })).toBeVisible();
        // Empty state
        await expect(page.getByText("No live sessions")).toBeVisible();

        await page.screenshot({
            path: shotPath("09-sessions-empty.png"),
            fullPage: false,
        });
    });
});
