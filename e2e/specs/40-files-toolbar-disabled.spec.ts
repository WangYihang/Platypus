import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The Files toolbar exposes Refresh / New / Upload / Download.
// Download is the only selection-dependent action — the rest are
// always available because they don't operate on the current
// selection. Rename / Chmod / Delete moved out of the toolbar and
// live exclusively in the right-click context menu now (the
// duplicated "More" dropdown was a redundant entry point).
test.describe("host files toolbar disabled state", () => {
    test("selection-dependent actions are disabled with no selection", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await page
            .getByTestId("fleet-panel-table")
            .locator("table tbody tr")
            .first()
            .click();
        await page.getByRole("tab", { name: "Files" }).click();
        await expect(page).toHaveURL(/\/files$/);

        // Wait for the directory listing.
        await expect(page.getByText("etc", { exact: true })).toBeVisible({
            timeout: 15_000,
        });

        const toolbar = page.getByTestId("files-toolbar");

        // Download requires at least one selection.
        await expect(
            toolbar.getByRole("button", { name: /^Download/ }),
        ).toBeDisabled();

        // Refresh / New / Upload don't depend on selection.
        for (const name of ["Refresh", "New", "Upload"]) {
            await expect(
                toolbar.getByRole("button", { name: new RegExp(`^${name}$`) }),
            ).toBeEnabled();
        }
    });
});
