import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host view", () => {
    test("click row lands on Info tab with header + tab strip", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/fleet(?:\?.*)?$/);

        // Click into the host row — Fleet Table view routes into
        // /hosts/:id/info (Terminal tab was extracted into the global
        // bottom drawer). Scope to the Table panel so the Timeline
        // panel's hidden session table doesn't resolve first.
        const row = page
            .getByTestId("fleet-panel-table")
            .locator("table tbody tr")
            .first();
        await expect(row).toBeVisible({ timeout: 10_000 });
        await row.click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/info$/);

        // PageHeader subtitle on HostView is "<N> active · <os>".
        await expect(
            page.getByText(/active · /).first(),
        ).toBeVisible({ timeout: 10_000 });

        // Current tab strip — Terminal moved to the global drawer and
        // Processes was added.
        for (const label of ["Info", "Files", "Processes"]) {
            await expect(page.getByRole("tab", { name: label })).toBeVisible();
        }
        // Sessions tab renders with a count suffix, match the prefix.
        await expect(
            page.getByRole("tab", { name: /^Sessions/ }),
        ).toBeVisible();
        // "Open terminal" button lives in the page header actions row.
        await expect(
            page.getByRole("button", { name: /Open terminal/i }),
        ).toBeVisible();

        await page.screenshot({
            path: shotPath("15-host-info.png"),
            fullPage: false,
        });
    });
});
