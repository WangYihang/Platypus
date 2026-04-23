import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host terminal", () => {
    test("terminal tab mounts xterm and handles command execution", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // xterm renders its prompt cells into `.xterm-rows`.
        const rows = page.locator(".xterm-rows");
        await expect(rows).toBeVisible({ timeout: 15_000 });

        // Let it settle.
        await page.waitForTimeout(1000);

        // Send a command to the terminal.
        // We type "whoami" and press Enter.
        await page.keyboard.type("whoami");
        await page.keyboard.press("Enter");

        // Wait for the output. The baseline agent in E2E runs as the current user.
        // In the dev environment, this is usually "ubuntu".
        await expect(rows).toContainText("ubuntu", { timeout: 5000 });

        // Also test directory listing.
        await page.keyboard.type("pwd");
        await page.keyboard.press("Enter");
        await expect(rows).toContainText("/", { timeout: 5000 });

        await page.screenshot({
            path: shotPath("16-host-terminal.png"),
            fullPage: false,
        });
    });
});
