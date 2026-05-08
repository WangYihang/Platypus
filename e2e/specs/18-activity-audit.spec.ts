import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("audit logs", () => {
    // Full coverage (exec command → activity row appears) needs the
    // terminal RPC path, which currently blocks on v2 not creating
    // session rows (see 11-host-terminal for the detail). We still
    // validate that the page routes cleanly and the header renders,
    // so a regression in either layer is caught early.
    //
    // 2026-05 IA pass: the consolidated "Audit" entry split back
    // out into a top-level "Activity" tab whose default sub-tab is
    // "sessions" (live + closed). Activities / Recordings /
    // Transfers are sibling sub-tabs at /activity/events,
    // /activity/recordings, /activity/transfers.
    test("Activity route defaults to Sessions tab", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /^Activity$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/activity\/sessions$/);
        await expect(page.getByRole("link", { name: /^Activity$/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("23-activity-audit.png"),
            fullPage: false,
        });
    });
});
