import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("audit logs", () => {
    test("terminal command execution generates an activity entry", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        
        // 1. Go to Terminal and run a command
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        const rows = page.locator(".xterm-rows");
        await expect(rows).toBeVisible({ timeout: 15_000 });
        await page.getByLabel("Terminal input").focus();
        await page.waitForTimeout(500);
        await page.keyboard.type("echo 'audit-test-command'", { delay: 50 });
        await page.keyboard.press("Enter");
        await expect(rows).toContainText("audit-test-command", { timeout: 10_000 });

        // 2. Go to Activities and verify the entry
        await page.getByRole("link", { name: /Activities$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/activities$/);
        
        // Look for an "exec" action in the list.
        // The list shows action type and the command in target_label.
        await expect(page.getByText("exec", { exact: true }).first()).toBeVisible({ timeout: 10_000 });
        await expect(page.getByText(/echo 'audit-test-command'/).first()).toBeVisible();

        await page.screenshot({
            path: shotPath("23-activity-audit.png"),
            fullPage: false,
        });
    });
});
