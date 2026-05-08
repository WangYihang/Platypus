import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("admin users", () => {
    test("manage users via the user menu", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: "User menu" }).click();
        await page.getByRole("button", { name: /Manage users/ }).click();
        await expect(page).toHaveURL(/\/admin\/users$/);

        // Admin row + role select. Scope to the admin-users table so
        // we don't pick up the "admin" copy in the sidebar pill.
        await expect(
            page.getByRole("cell", { name: "admin" }).first(),
        ).toBeVisible();
        await expect(page.getByRole("button", { name: /New user/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("12-admin-users.png"),
            fullPage: false,
        });
    });
});
