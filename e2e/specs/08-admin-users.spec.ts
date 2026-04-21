import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("admin users", () => {
    test("manage users via the user menu", async ({ page }) => {
        await loginAsAdmin(page);
        // Open user popover via the more-icon button at the bottom of
        // the sidebar. /projects has the sidebar visible without
        // selecting a project first.
        await page.getByRole("button", { name: "User menu" }).click();
        await page.getByRole("button", { name: /Manage users/ }).click();
        await expect(page).toHaveURL(/\/admin\/users$/);

        // Admin row + role select
        await expect(page.getByText("admin").first()).toBeVisible();
        await expect(page.getByRole("button", { name: /New user/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("12-admin-users.png"),
            fullPage: false,
        });
    });
});
