import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("audit logs", () => {
    // Full coverage (exec command → activity row appears) needs the
    // terminal RPC path, which currently blocks on v2 not creating
    // session rows (see 11-host-terminal for the detail). We still
    // validate that the page routes cleanly and the header renders,
    // so a regression in either layer is caught early.
    test("Activities route renders the audit feed page shell", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Activities$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/activities$/);
        await expect(page.getByRole("link", { name: /Activities$/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("23-activity-audit.png"),
            fullPage: false,
        });
    });
});
