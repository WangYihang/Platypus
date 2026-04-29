import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// Listeners and Dispatch were removed in the v2 IA refactor (Fleet
// merged Hosts/Sessions/Topology, Dispatch deleted entirely). Several
// page subtitles, empty-state descriptions and call-to-action labels
// still mentioned the dead concepts. This regression guard scans the
// surfaces that carried the stale wording.
test.describe("no stale 'listeners' or 'dispatch' copy", () => {
    test("Projects landing subtitle and empty-state mention neither", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects$/);
        await expect(page.getByText("Projects", { exact: true })).toBeVisible();

        const text =
            (await page.locator('main, [role="main"]').first().textContent()) ?? "";
        expect(text).not.toMatch(/\blisteners?\b/i);
        expect(text).not.toMatch(/\bdispatch(es)?\b/i);
    });

    test("Fleet empty-state on a project with no agents", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Staging created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        // Fleet ships two panels (cards + table), both render the
        // empty state — scope to the table panel so the assertion
        // doesn't strict-mode-fail on duplicate matches.
        await expect(
            page.getByTestId("fleet-panel-table").getByText("No hosts yet"),
        ).toBeVisible();

        const main = page.locator("main");
        const text = (await main.textContent()) ?? "";
        expect(text).not.toMatch(/\blisteners?\b/i);
    });
});
