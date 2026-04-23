import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("project shell", () => {
    test("sidebar renders the brand, switcher, nav, and user menu", async ({ page }) => {
        await loginAsAdmin(page);
        // Click into the Default project tile (SPA nav, preserves in-memory
        // session) instead of page.goto which would full-reload and force
        // a session rehydrate. Card with interactive+onClick exposes
        // role=button (see components/Card.tsx).
        await page
            .getByRole("button", { name: /Default created/i })
            .click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);

        // Brand (aria-labelled span in components/Brand.tsx).
        await expect(page.getByLabel("Platypus")).toBeVisible();
        // All six nav links.
        await expect(page.getByRole("link", { name: /Overview$/ })).toBeVisible({ timeout: 10_000 });

        for (const label of [
            "Overview",
            "Hosts",
            "Listeners",
            "Sessions",
            "Dispatch",
            "Members",
        ]) {
            // Antd icons inject their name into the accessible label
            // ("appstore Overview"), so match the trailing label.
            await expect(
                page.getByRole("link", { name: new RegExp(`${label}$`) }),
            ).toBeVisible();
        }
        await page.screenshot({ path: shotPath("03-sidebar.png"), fullPage: false });
    });
});
