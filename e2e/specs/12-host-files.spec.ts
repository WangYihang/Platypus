import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host files", () => {
    test("Files tab renders directory listing and handles navigation", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // Wait for xterm + tabs to settle.
        await expect(page.locator(".xterm-rows")).toBeVisible({ timeout: 15_000 });
        await page.waitForTimeout(1000);
        await page.getByRole("tab", { name: "Files" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);

        // Toolbar buttons and the initial directory listing render.
        await expect(page.getByRole("button", { name: /New folder/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /Upload/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /Refresh/ }).last()).toBeVisible();

        // Should see the root directory entries.
        await expect(page.getByText("etc", { exact: true })).toBeVisible({ timeout: 10_000 });

        // Navigate into /etc.
        await page.getByRole("button", { name: "etc" }).click();
        
        // Should see entries inside /etc.
        await expect(page.getByText("hostname", { exact: true })).toBeVisible({ timeout: 10_000 });

        await page.screenshot({
            path: shotPath("17-host-files.png"),
            fullPage: false,
        });
    });
});
