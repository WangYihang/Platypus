import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("projects multi", () => {
    test("project switcher dropdown lists both projects", async ({ page }) => {
        await loginAsAdmin(page);
        // Enter a project so the switcher shows a current project.
        await page.getByRole("button", { name: /Default created/i }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);

        // Click the project switcher in the sidebar.
        await page.getByRole("button", { name: /Default default down/i }).click();
        // Both projects appear in the popover.
        const popover = page.getByText("Projects", { exact: true }).first();
        await expect(popover).toBeVisible();
        await expect(page.getByRole("button", { name: /Default/ }).first())
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
