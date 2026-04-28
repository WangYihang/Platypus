import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The Files chrome shrank to a single breadcrumb row: ↑ + ⟳ + path
// crumbs + QuickPaths chips. Every other action moved into the
// right-click context menu (operators reported the previous toolbar
// duplicated the menu's items). This spec pins the new contract:
//
//   1. Refresh stays as a one-click icon next to the up-arrow.
//   2. Right-clicking an empty area of the file pane surfaces
//      New file / New folder / Upload here / Refresh.
//   3. Right-clicking a row surfaces Open + Copy path + Delete (the
//      menu's load-bearing items that exist regardless of what's
//      selected).
//
// We deliberately do not assert text-labelled toolbar buttons here —
// they're gone, and a regression would re-introduce the duplication
// the operator complained about.
test.describe("host files chrome + right-click menu contract", () => {
    test.beforeEach(async ({ page }) => {
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
        // Wait for the listing — without it the right-click target
        // is racing against the agent's first /fs/list response.
        await expect(page.getByText("etc", { exact: true })).toBeVisible({
            timeout: 15_000,
        });
    });

    test("breadcrumb chrome exposes a Refresh icon button", async ({ page }) => {
        const refresh = page.getByTestId("files-refresh");
        await expect(refresh).toBeVisible();
        await expect(refresh).toBeEnabled();
        // The icon-only button has its name via aria-label.
        await expect(refresh).toHaveAttribute("aria-label", "Refresh");
    });

    test("right-click on empty area opens the new/upload/refresh menu", async ({
        page,
    }) => {
        // Click the breadcrumb row to clear any incidental row
        // selection, then right-click on the file-pane background.
        // The empty-variant menu mounts on the .rounded-md.border
        // wrapper we registered the FileContextMenu against.
        const pane = page.getByTestId("files-breadcrumb-row");
        await pane.click();

        // Right-click well below the last row to land on empty space.
        const fileArea = page.locator('[data-slot="table-container"]').first();
        const box = await fileArea.boundingBox();
        if (!box) throw new Error("file pane has no bounding box");
        await page.mouse.move(box.x + box.width / 2, box.y + box.height - 8);
        await page.mouse.click(box.x + box.width / 2, box.y + box.height - 8, {
            button: "right",
        });

        for (const item of ["New file", "New folder", "Upload here", "Refresh"]) {
            await expect(
                page.getByRole("menuitem", { name: new RegExp(`^${item}$`) }),
            ).toBeVisible();
        }
        // Close the menu so the next test starts clean.
        await page.keyboard.press("Escape");
    });

    test("right-click on a row exposes Open / Copy path / Delete", async ({
        page,
    }) => {
        const row = page.getByText("etc", { exact: true });
        await row.click({ button: "right" });

        for (const item of ["Open", "Copy path", "Delete"]) {
            await expect(
                page.getByRole("menuitem", { name: new RegExp(`^${item}$`) }),
            ).toBeVisible();
        }
        await page.keyboard.press("Escape");
    });
});
