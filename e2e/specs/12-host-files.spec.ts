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

        // Form + the three action buttons render.
        await expect(page.getByText("Transfer")).toBeVisible();
        await expect(page.getByLabel("Remote path")).toBeVisible();
        await expect(page.getByRole("button", { name: "Get size" })).toBeVisible();
        await expect(page.getByRole("button", { name: /Download/ })).toBeVisible();
        await expect(page.getByRole("button", { name: /Upload/ })).toBeVisible();

        // Pre-fill the path so the screenshot shows what the user
        // would type, then snapshot. The actual API roundtrip is
        // covered by the FileSize backend test, not the e2e gallery.
        await page.getByLabel("Remote path").click();
        await page.getByLabel("Remote path").pressSequentially("/etc/hostname");

        await page.screenshot({
            path: shotPath("17-host-files.png"),
            fullPage: false,
        });
    });
});
