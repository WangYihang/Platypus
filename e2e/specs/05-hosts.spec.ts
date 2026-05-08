import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

// Hosts is the project's host inventory. Default lens is the table
// (cards is the alternative); toggle is preserved per-user.
test.describe("hosts · table view", () => {
    test("populated list shows the baseline agent's host", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts(?:\?.*)?$/);

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
