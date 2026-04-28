import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Empty-state coverage: staging has no listener and no agent, so its
// Fleet table view renders the "No hosts yet" empty state. Default
// project is now populated thanks to the baseline agent in
// globalSetup.
test.describe("empty project", () => {
    test("staging fleet page shows the empty state CTA", async ({ page }) => {
        await loginAsAdmin(page);

        // Click the Staging tile directly from the landing grid.
        await page.getByRole("button", { name: /Staging created/i }).click();
        await expect(page).toHaveURL(/\/projects\/staging\/overview$/);

        await page.getByRole("link", { name: /Fleet$/ }).click();
        await expect(page).toHaveURL(/\/projects\/staging\/fleet(?:\?.*)?$/);

        await expect(page.getByText("No hosts yet")).toBeVisible();
        // Step 1 of the settings reorg renamed the empty-state CTA from
        // "Manage enrollment" to "Enroll agent". The page header on
        // Fleet now also surfaces an "Enroll agent" link, so the empty
        // state may match more than one element — scope to the empty
        // state's own action button.
        await expect(
            page
                .getByRole("button", { name: /Enroll agent/i })
                .first(),
        ).toBeVisible();

        await page.screenshot({
            path: shotPath("21-hosts-empty.png"),
            fullPage: false,
        });
    });
});
