import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Hosts is now the default Table view of the Fleet page. Old
// standalone /hosts route is gone; any navigation from the sidebar
// lands on /fleet and the Table toggle is selected by default.
test.describe("fleet · hosts (table view)", () => {
    test("populated list shows the baseline agent's host", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/fleet(?:\?.*)?$/);

        // baseline agent registered in globalSetup → exactly 1 host row.
        // Scope to the Table panel — Timeline and Graph panels stay
        // mounted (display:none) so their tables would otherwise count.
        const table = page.getByTestId("fleet-panel-table");
        const rows = table.locator("table tbody tr");
        await expect(rows).toHaveCount(1, { timeout: 10_000 });

        // Toolbar shows the summary ("1 total · 1 online").
        await expect(table.getByText(/1 total · 1 online/)).toBeVisible();

        await page.screenshot({
            path: shotPath("08-hosts-list.png"),
            fullPage: false,
        });
    });
});
