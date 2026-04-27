import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// "Listener" is a v1 concept that has been removed from the user-facing
// surface (commit 977a3fa scrubbed it from the rail / quick actions /
// hosts empty state), but two references survived inside the Sessions
// panel: a search-box placeholder and the "all sessions" empty-state
// description. New users see a noun that is defined nowhere else in the
// app and try to find a Listeners page.
//
// Lock both surfaces to a vocabulary that matches HostsPanel's
// "agent enrolls into this project" wording so the two screens read as
// describing one product, not two.
test.describe("sessions panel — no legacy 'listener' copy", () => {
    test("placeholder + empty state never mention 'listener'", async ({ page }) => {
        await loginAsAdmin(page);

        // Drop into the default project's Fleet → Timeline view (the
        // Sessions panel; the URL param is `timeline` even though the
        // panel itself is the Sessions UI).
        await page.goto("/projects/default/fleet?view=timeline");

        // The search input's placeholder must not include "listener".
        const search = page.locator('input[placeholder*="Search session"]');
        await expect(search).toBeVisible({ timeout: 10_000 });
        const placeholder = await search.getAttribute("placeholder");
        expect(placeholder ?? "").not.toMatch(/listener/i);

        // No EmptyState description anywhere on the panel may say
        // "listener" / "listeners". Walk the whole rendered DOM as a
        // belt-and-braces check; cheaper than locating each empty
        // variant. (filter=live and filter=all surface different copy
        // — only one is on screen at a time, but neither variant should
        // ever mention listeners.)
        const liveCopy = (await page.locator("body").textContent()) ?? "";
        expect(liveCopy).not.toMatch(/listener/i);

        const allToggle = page.getByRole("button", { name: /^all$/i }).first();
        if (await allToggle.isVisible().catch(() => false)) {
            await allToggle.click();
            const allCopy = (await page.locator("body").textContent()) ?? "";
            expect(allCopy).not.toMatch(/listener/i);
        }
    });
});
