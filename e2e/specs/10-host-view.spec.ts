import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host view", () => {
    test("click row lands on Info tab with header + tab strip", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts(?:\?.*)?$/);

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
        // Hop over to the Info activity — the rest of this spec asserts
        // on the host header + activity bar, which renders identically
        // regardless of the active activity. The host tab strip moved
        // to a vertical ActivityBar with one <button> per entry; each
        // button carries data-testid="host-activity-<activityKey>".
        await page.getByTestId("host-activity-info").click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/info$/);

        // Current activity bar — Terminal moved to the global drawer.
        for (const key of ["info", "files", "processes", "sessions"]) {
            await expect(page.getByTestId(`host-activity-${key}`)).toBeVisible();
        }
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
