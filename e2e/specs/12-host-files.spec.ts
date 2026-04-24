import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host files", () => {
    // Real directory listing works against the baseline agent, so
    // we assert the contents render, not just the toolbar. Same
    // wiring concern as 11-host-terminal: a regression to using the
    // sessions-row UUID as the route param would surface here as the
    // "Load error: 404: agent ... not connected" banner.
    test("Files tab loads root directory entries from the agent", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await page
            .getByTestId("fleet-panel-table")
            .locator("table tbody tr")
            .first()
            .click();
        await page.getByRole("tab", { name: "Files" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);

        // /etc is on every Linux agent; if the listing rendered at
        // all the Files tab is fully wired (PAT enroll → link →
        // /api/v1/agents/:id/fs/list keyed on agent_id).
        await expect(page.getByText("etc", { exact: true })).toBeVisible({ timeout: 15_000 });
        // Negative: the precise error string the SSOT-bug regression
        // produced.
        await expect(page.getByText(/Load error.*not connected/i)).not.toBeVisible();

        await page.screenshot({
            path: shotPath("17-host-files.png"),
            fullPage: false,
        });
    });
});
