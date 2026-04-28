import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("audit logs", () => {
    // Full coverage (exec command → activity row appears) needs the
    // terminal RPC path, which currently blocks on v2 not creating
    // session rows (see 11-host-terminal for the detail). We still
    // validate that the page routes cleanly and the header renders,
    // so a regression in either layer is caught early.
    //
    // 2026-04 IA pass: the three audit surfaces (Activities /
    // Recordings / Transfers) consolidated under a single "Audit"
    // sidebar entry that opens AuditPage with internal tabs. The
    // canonical landing URL is now /projects/<slug>/audit/activities.
    test("Audit route lands on Activities tab", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /^Audit$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/audit\/activities$/);
        // The sidebar entry stays highlighted on every audit/* URL.
        await expect(page.getByRole("link", { name: /^Audit$/ })).toBeVisible();
        // The page-level header now reads "Audit"; the active tab
        // reads "Activities" inside the right-aligned tab strip.
        await expect(page.getByRole("tab", { name: /Activities/ })).toHaveAttribute(
            "data-state",
            "active",
        );

        await page.screenshot({
            path: shotPath("23-activity-audit.png"),
            fullPage: false,
        });
    });
});
