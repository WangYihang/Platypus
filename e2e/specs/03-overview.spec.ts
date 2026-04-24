import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("project overview dashboard", () => {
    test("KPIs + charts render", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);

        // Core KPI tiles. Listeners KPI was removed with the listener
        // concept; we check the surviving three.
        for (const label of ["Hosts", "Online now", "Live sessions"]) {
            await expect(page.getByText(label, { exact: true }).first()).toBeVisible();
        }

        // Chart cards still rendering.
        await expect(page.getByText("Sessions (last 24h)")).toBeVisible();
        await expect(page.getByText("Top hosts (24h)")).toBeVisible();

        await page.screenshot({
            path: shotPath("04-project-overview.png"),
            fullPage: true,
        });
    });
});
