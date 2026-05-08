import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// F10: the Enrollment dialogs used to ship raw "TTL (seconds)" /
// "Binding machine ID" labels. Both required the user to know
// implementation jargon — bare seconds (3600 = ?), or the existence
// of /etc/machine-id — to make safe choices. The renamed copy:
//
//   · "Download link TTL (seconds)" → "Expires in (seconds)" with a
//     live "= 5m" / "= 1d 2h" conversion shown inline as the user
//     types so they don't have to do the arithmetic.
//   · "TTL (seconds)" (PAT dialog) → same.
//   · "Binding machine ID" → "Restrict to machine" + a description
//     that says WHY you would set this and what happens if you don't.
//
// This spec opens both dialogs, asserts the renamed labels, and types
// a known number into the TTL field to confirm the conversion helper
// renders in real time.
test.describe("enrollment dialogs — friendlier TTL + machine-id labels", () => {
    test("install-command dialog: 'Expires in' label + live conversion", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/enrollment");

        // Open the install-command dialog.
        await page
            .getByRole("button", { name: /generate install command/i })
            .click();
        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();

        // Label rename — old "TTL (seconds)" gone, new "Expires in" present.
        const dialogText = (await dialog.textContent()) ?? "";
        expect(dialogText, "dialog still uses 'TTL'").not.toMatch(/\bTTL\b/);
        expect(dialogText).toMatch(/expires in/i);

        // Type 86400 (one day) and assert "= 1d" appears in the dialog.
        const input = dialog.getByLabel(/expires in/i);
        await input.fill("86400");
        const live1 = (await dialog.textContent()) ?? "";
        expect(live1).toMatch(/=\s*1d/);

        // Change to 90 (1m 30s) to confirm the helper updates as the
        // value changes.
        await input.fill("90");
        const live2 = (await dialog.textContent()) ?? "";
        expect(live2).toMatch(/=\s*1m\s*30s/);
    });

    test("enrollment-token dialog: 'Expires in' + 'Restrict to machine' wording", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/enrollment");
        await page.getByRole("tab", { name: /enrollment tokens/i }).click();
        await page
            .getByRole("button", { name: /issue (an? )?enrollment token/i })
            .click();

        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();

        const dialogText = (await dialog.textContent()) ?? "";

        // Same TTL rename.
        expect(dialogText, "dialog still uses 'TTL'").not.toMatch(/\bTTL\b/);
        expect(dialogText).toMatch(/expires in/i);

        // Machine-id rename.
        expect(dialogText, "old 'Binding machine ID' label survives").not.toMatch(
            /binding machine id/i,
        );
        expect(dialogText).toMatch(/restrict to machine/i);

        // Live conversion still works on this dialog too. 3600 → "1h".
        await dialog.getByLabel(/expires in/i).fill("3600");
        const live = (await dialog.textContent()) ?? "";
        expect(live).toMatch(/=\s*1h/);
    });
});
