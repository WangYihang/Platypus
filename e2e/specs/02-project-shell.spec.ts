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

        // Server switcher sits at the top of the sidebar (the
        // standalone server rail column was retired in 2026-04 IA pass).
        // Sidebar starts collapsed to an icon-only rail; click
        // through the chevron so the text labels assertions below
        // resolve.
        await page.getByRole("button", { name: /Expand sidebar/i }).click();
        await expect(page.getByTestId("server-switcher-trigger")).toBeVisible();
        await expect(page.getByRole("link", { name: /Overview$/ })).toBeVisible({ timeout: 10_000 });

        // Current nav surface (desktop/frontend/src/layout/ProjectSidebar.tsx).
        // Activities + Recordings collapsed into a single Audit entry;
        // Enrollment moved inside Fleet (it's how you grow the fleet);
        // Settings is admin-only (we're logged in as admin so it
        // renders).
        for (const label of ["Overview", "Fleet", "Members", "Audit", "Settings"]) {
            await expect(
                page.getByRole("link", { name: new RegExp(`${label}$`) }),
            ).toBeVisible();
        }
        await page.screenshot({ path: shotPath("03-sidebar.png"), fullPage: false });
    });
});
