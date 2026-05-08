import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// 2026-05 IA pass: the user-menu "Manage users" button moved to a
// global "Admin" tab in the top bar (visible when no project is in
// scope). Admin-role users see it; the Admin route lands on
// /admin/users by default.
test.describe("admin users", () => {
    test("manage users via the Admin top-bar tab", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("link", { name: /^Admin$/ }).click();
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
