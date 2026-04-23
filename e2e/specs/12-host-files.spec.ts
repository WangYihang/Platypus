import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host files", () => {
    test("Files tab renders the path-based transfer form", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // Wait for xterm + tabs to settle (StrictMode double-mounts xterm).
        await expect(page.locator(".xterm-rows")).toBeVisible({ timeout: 15_000 });
        await page.waitForTimeout(800);
        await page.getByRole("tab", { name: "Files" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);

        // Toolbar buttons and the initial directory listing render.
        await expect(page.getByRole("button", { name: /New folder/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /Upload/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /Download/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /Refresh/ }).last()).toBeVisible();

        // Should see the root directory entries (or "Empty directory" if new).
        await expect(
            page.getByText(/entries|Empty directory/),
        ).toBeVisible();

        await page.screenshot({
            path: shotPath("17-host-files.png"),
            fullPage: false,
        });
    });
});
