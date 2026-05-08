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

        // Click into the host row — Fleet Table view routes into the
        // host's default tab (Files since 9748a49 made Files the
        // landing tab). Scope the selector to the Table panel so the
        // Timeline panel's hidden session table doesn't resolve
        // first.
        const row = page
            .getByTestId("fleet-panel-table")
            .locator("table tbody tr")
            .first();
        await expect(row).toBeVisible({ timeout: 10_000 });
        await row.click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);
        // Hop over to the Info tab — the rest of this spec asserts on
        // the host header + tab strip, which renders identically
        // regardless of the active tab.
        await page.getByRole("tab", { name: "Info" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/info$/);

        // PageHeader subtitle on HostView is "<N> active · <os>".
        await expect(
            page.getByText(/active · /).first(),
        ).toBeVisible({ timeout: 10_000 });

        // Current tab strip — Terminal moved to the global drawer.
        for (const label of ["Info", "Files", "Processes"]) {
            await expect(page.getByRole("tab", { name: label })).toBeVisible();
        }
        // Sessions tab renders with a count suffix, match the prefix.
        await expect(
            page.getByRole("tab", { name: /^Sessions/ }),
        ).toBeVisible();
        // "Open terminal" button lives in the page header actions
        // row. Match the exact name so the status-bar's terminals
        // pill ("N open terminal(s)") doesn't double-resolve.
        await expect(
            page
                .getByTestId("shell-content-frame")
                .getByRole("button", { name: "Open terminal" }),
        ).toBeVisible();

        await page.screenshot({
            path: shotPath("15-host-info.png"),
            fullPage: false,
        });
    });
});
