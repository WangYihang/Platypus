import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The Files toolbar exposes Download / Rename / Chmod / Delete —
// all selection-dependent. With no row selected they must be
// disabled (visually faded + onClick a no-op) so users don't try
// the action and get a silent failure or a confusing toast.
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

        // No row selected; the toolbar buttons that need exactly
        // one selection (Download / Rename / Chmod) and the Delete
        // button (>=1) are disabled.
        for (const name of ["Download", "Rename", "Chmod", "Delete"]) {
            await expect(
                toolbar.getByRole("button", { name: new RegExp(`^${name}$`) }),
            ).toBeDisabled();
        }

        // Refresh / New folder / Upload don't depend on selection.
        for (const name of ["Refresh", "New folder", "Upload"]) {
            await expect(
                toolbar.getByRole("button", { name: new RegExp(`^${name}$`) }),
            ).toBeEnabled();
        }
    });
});
