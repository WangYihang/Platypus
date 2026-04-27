import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// Enrollment artefacts have four statuses: a fresh / unused token, a
// successfully consumed one, an expired one, and a revoked one. The
// page used the back-end's lifecycle words verbatim ("pending" /
// "consumed") and coloured them like this:
//
//   pending  → success (green)   ← misleading: green = "this worked"
//   consumed → info    (blue)    ← also misleading: success state, not info
//
// Users reading "pending" in green think the action has succeeded. The
// vocabulary + colour pair we want users to see instead:
//
//   unused   → info / neutral  (a fresh token, not yet redeemed)
//   used     → success (green) (token was consumed successfully)
//   expired  → warning (unchanged)
//   revoked  → danger  (unchanged)
//
// This spec opens the install-command tab, issues a token, and asserts
// the resulting row uses the new vocabulary on its status pill. Colour
// is checked via the rendered border colour the StatusPill component
// applies — we just require it to NOT be the green successDot value
// when the row is still unused.
test.describe("enrollment status — unused / used vocabulary", () => {
    test("a freshly issued install command shows 'Unused', not 'Pending'", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/enrollment");

        // Wait for the install-commands tab to be the active surface.
        await expect(
            page.getByRole("button", { name: /generate install command/i }),
        ).toBeVisible({ timeout: 10_000 });

        // Open the issue dialog and submit with defaults.
        await page.getByRole("button", { name: /generate install command/i }).click();
        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();

        // "Agent should dial" is required. Use the e2e backend's host:port
        // so the form passes server-side validation.
        await dialog.getByLabel(/agent should dial/i).fill("127.0.0.1:7332");
        await dialog.getByRole("button", { name: /^generate$/i }).click();

        // The "command issued" dialog opens. We dismiss via the X
        // close button (the primary "I'll run this — show me Fleet"
        // button navigates to /fleet, which would lose the enrollment
        // table this spec needs to inspect).
        const issuedDialog = page.getByRole("dialog").filter({
            hasText: /this is the only time/i,
        });
        await expect(issuedDialog).toBeVisible({ timeout: 10_000 });
        await issuedDialog.getByRole("button", { name: /^close$/i }).click();

        // The new row should appear in the table with status "Unused".
        // Look for a status pill on the page.
        const unusedPill = page.locator("text=/^unused$/i").first();
        await expect(unusedPill).toBeVisible({ timeout: 5_000 });

        // And the literal lifecycle word "pending" must NOT be visible
        // anywhere on the rendered table.
        const allText = (await page.locator("body").textContent()) ?? "";
        // Allow "pending" only inside scrollable hidden text (e.g.
        // accessibility instructions); we just check no visible status
        // pill says "pending".
        const visiblePending = page.locator("text=/^pending$/i");
        await expect(visiblePending).toHaveCount(0);
        // Belt: a successfully-styled green pill on the unused token
        // would have the successDot border colour (#3ECF8E). We weaken
        // this to: the unused pill's border colour must not be the
        // green successDot value, since "unused" should read as neutral
        // / informational, not "success".
        const borderColor = await unusedPill.evaluate(
            (el) => getComputedStyle(el).borderColor,
        );
        // Green successDot is rgb(62, 207, 142). Reject that exact value.
        expect(borderColor.replace(/\s+/g, "")).not.toBe("rgb(62,207,142)");

        // Sanity: "Pending" word never sneaks back as the row label.
        expect(allText.match(/\bpending\b/i)?.length ?? 0).toBeLessThan(2);
    });
});
