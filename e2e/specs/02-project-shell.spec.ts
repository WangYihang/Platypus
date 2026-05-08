import { expect, test } from "../fixtures/test";

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

        // Server switcher lives in the top bar (the standalone server
        // rail column was retired in 2026-04 IA pass).
        await expect(page.getByTestId("server-switcher-trigger")).toBeVisible();
        await expect(page.getByRole("link", { name: /Overview$/ })).toBeVisible({ timeout: 10_000 });

        // Current nav surface (desktop/frontend/src/layout/NavTabs.tsx).
        // Fleet was renamed to Hosts in the IA split that promoted
        // Activity rollups into their own surface; Security is its own
        // tab (was inside the project surface). Settings is
        // admin-only (we're logged in as admin so it renders).
        for (const label of [
            "Overview",
            "Hosts",
            "Activity",
            "Security",
            "Enrollment",
            "Members",
            "Settings",
        ]) {
            await expect(
                page.getByRole("link", { name: new RegExp(`${label}$`) }),
            ).toBeVisible();
        }
        await page.screenshot({ path: shotPath("03-sidebar.png"), fullPage: false });
    });
});
