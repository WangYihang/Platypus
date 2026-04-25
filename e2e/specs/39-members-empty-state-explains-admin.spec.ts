import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// Visiting /members as a global admin shows "0 members" plus a copy
// like "Add a user to grant them access" — but the admin clearly has
// access (they're reading the page). The discrepancy reads as a bug
// ("did my account get demoted?"). Spell out that global admins
// access every project regardless of explicit project membership.
test.describe("members empty state", () => {
    test("explains that global admins have implicit access", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Members$/ }).click();
        await expect(page).toHaveURL(/\/members$/);

        await expect(page.getByText("No members")).toBeVisible();
        const main = page.locator("main");
        const text = (await main.textContent()) ?? "";
        // Either "global admin" wording (canonical) or any
        // explanation of admin's implicit access. Be loose so the
        // copy can iterate without breaking the test.
        expect(text).toMatch(/global admin/i);
    });
});
