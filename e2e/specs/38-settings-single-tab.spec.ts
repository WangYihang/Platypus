import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// ProjectSettings has only one tab today ("General"). Showing the
// shadcn TabsList strip with a single trigger is just visual noise —
// it implies more sections exist. Hide the strip when there is only
// one tab.
test.describe("project settings tab strip", () => {
    test("TabsList is not rendered when only one tab", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Settings$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/settings$/);

        // Tab content (the General body) is still rendered.
        await expect(page.getByText("Identity", { exact: true })).toBeVisible();

        // But the TabsList itself isn't.
        await expect(page.getByRole("tablist")).toHaveCount(0);
    });
});
