import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Dispatch was removed in the UI refactor. The members surface stays
// under /projects/:slug/members; keep its coverage here so the file
// name lines up with the existing screenshot gallery entry.
test.describe("members", () => {
    test("members page lists the admin", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Members$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/members$/);

        // Admin is auto-added as a member of any project they create.
        await expect(page.getByText("admin").first()).toBeVisible();
        await expect(page.getByRole("button", { name: /Add member/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("11-members.png"),
            fullPage: false,
        });
    });
});
