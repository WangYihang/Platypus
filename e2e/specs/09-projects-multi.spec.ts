import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("projects multi", () => {
    test("project switcher dropdown lists both projects", async ({ page }) => {
        await loginAsAdmin(page);
        // Enter a project so the switcher shows a current project.
        await page.getByRole("button", { name: /Default created/i }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);

        // Sidebar collapses to an icon-only rail by default and the
        // ServerSwitcher / ProjectSwitcher are hidden in that mode
        // (Cmd-K is the surface for switching while collapsed).
        // Expand so the switcher trigger is in the DOM.
        await page.getByRole("button", { name: /Expand sidebar/i }).click();

        // Click the project switcher in the sidebar. Scope to the
        // sidebar (`role=complementary`) so we don't pick up the
        // "Default" tile copy on the overview page.
        await page
            .getByRole("complementary")
            .getByRole("button", { name: /Default/ })
            .first()
            .click();

        // Wait for popover animation.
        await page.waitForTimeout(500);

        // Both projects appear in the popover.
        await expect(page.getByText("Projects", { exact: true }).first()).toBeVisible();
        await expect(page.getByRole("button", { name: /Default/ }).last())
            .toBeVisible();
        await expect(page.getByRole("button", { name: /Staging/ }))
            .toBeVisible();
        await page.screenshot({
            path: shotPath("13-project-switcher-open.png"),
            fullPage: false,
        });
    });

    test("projects landing tile grid", async ({ page }) => {
        await loginAsAdmin(page);
        // Already on /projects. Each tile renders the project name in
        // both the slug chip and the headline, so use first() rather
        // than expect the locator to be unique.
        await expect(page.getByText("Default", { exact: true }).first())
            .toBeVisible();
        await expect(page.getByText("Staging", { exact: true }).first())
            .toBeVisible();
        await page.screenshot({
            path: shotPath("14-projects-landing-multi.png"),
            fullPage: false,
        });
    });
});
