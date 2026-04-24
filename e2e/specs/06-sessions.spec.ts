import { expect, test } from "../fixtures/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Sessions is now the Timeline view of the Fleet page. Toggling to
// Timeline writes ?view=timeline and shows the Live/All filter chips.
test.describe("fleet · sessions (timeline view)", () => {
    test("Timeline view renders with Live / All filter chips", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/fleet(?:\?.*)?$/);

        // Switch to Timeline. ToggleGroupItem renders a radio-role
        // button with the label text.
        await page.getByRole("radio", { name: /Timeline/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/fleet\?view=timeline$/);

        // Filter chips always render regardless of rows.
        await expect(page.getByText("Live", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("All", { exact: true })).toBeVisible();

        await page.screenshot({
            path: shotPath("09-sessions-list.png"),
            fullPage: false,
        });
    });
});
