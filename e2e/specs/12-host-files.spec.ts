import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host files", () => {
    // Same v2 caveat as 11-host-terminal: FilesTab needs a live
    // session to list the remote filesystem, and v2 doesn't create
    // session rows on agent connect. We still exercise the tab's
    // routing + toolbar shell so a regression in those layers is
    // caught here; the directory-browse coverage reactivates once
    // the server starts auto-creating sessions.
    test("Files tab routes to /files and toolbar renders", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await page.getByRole("tab", { name: "Files" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);
        await expect(page.getByRole("tab", { name: "Files", selected: true })).toBeVisible();

        await page.screenshot({
            path: shotPath("17-host-files.png"),
            fullPage: false,
        });
    });
});
