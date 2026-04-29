import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Dispatch was removed in the UI refactor. The members surface stays
// under /projects/:slug/members; keep its coverage here so the file
// name lines up with the existing screenshot gallery entry.
test.describe("members", () => {
    test("members page renders the empty-state explainer", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Members$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/members$/);

        // Global admins (the seeded user is one) implicitly see every
        // project — they aren't enrolled as project members. The page
        // explains that and ships the "Add member" CTA so the operator
        // can grant Operator / Viewer access to a non-admin user.
        await expect(page.getByText(/No members/i)).toBeVisible();
        await expect(
            page.getByText(/Global admins.*don.?t need to be members/i),
        ).toBeVisible();
        await expect(page.getByRole("button", { name: /Add member/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("11-members.png"),
            fullPage: false,
        });
    });
});
