import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("project overview dashboard", () => {
    test("KPIs + charts + listeners + activity render", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);

        // 4 KPI tiles
        for (const label of ["Hosts", "Online now", "Listeners", "Live sessions"]) {
            await expect(page.getByText(label, { exact: true }).first()).toBeVisible();
        }

        // Sessions chart card
        await expect(page.getByText("Sessions (last 24h)")).toBeVisible();
        await expect(page.getByText("Top hosts (24h)")).toBeVisible();

        // Listeners mini-list shows the seeded listener
        await expect(page.getByText("127.0.0.1:13399")).toBeVisible();

        // Quick actions card
        await expect(page.getByText("Quick actions")).toBeVisible();
        await expect(page.getByText("Create a listener")).toBeVisible();
        await expect(page.getByText("Run dispatch")).toBeVisible();

        await page.screenshot({
            path: shotPath("04-project-overview.png"),
            fullPage: true,
        });
    });
});
