import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("hosts", () => {
    test("populated list shows the baseline agent's host", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts$/);

        // baseline agent registered in globalSetup → exactly 1 host row.
        const rows = page.locator("table tbody tr");
        await expect(rows).toHaveCount(1, { timeout: 10_000 });

        // Subtitle reports counts ("1 total · 1 online").
        await expect(page.getByText(/1 total · 1 online/)).toBeVisible();

        await page.screenshot({
            path: shotPath("08-hosts-list.png"),
            fullPage: false,
        });
    });
});
