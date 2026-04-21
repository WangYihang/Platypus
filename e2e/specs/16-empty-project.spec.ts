import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Empty-state coverage moved here: staging has no listener and no agent,
// so its Hosts page renders the empty state. The default project is now
// populated thanks to the baseline agent in globalSetup.
test.describe("empty project", () => {
    test("staging hosts page shows the empty state CTA", async ({ page }) => {
        await loginAsAdmin(page);

        // Click the Staging tile directly from the landing grid.
        await page.getByRole("button", { name: /Staging created/i }).click();
        await expect(page).toHaveURL(/\/projects\/staging\/overview$/);

        await page.getByRole("link", { name: /Hosts$/ }).click();
        await expect(page).toHaveURL(/\/projects\/staging\/hosts$/);

        await expect(page.getByText("No hosts yet")).toBeVisible();
        await expect(page.getByRole("button", { name: /Manage listeners/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("21-hosts-empty.png"),
            fullPage: false,
        });
    });
});
