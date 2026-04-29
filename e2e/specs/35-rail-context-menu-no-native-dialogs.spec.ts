import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The original server rail used window.prompt() to rename and
// window.confirm() to remove — both render OS-native gray alert
// chrome that clashed with the dark-themed app. The 2026-04 IA pass
// folded the rail into a sidebar dropdown (ServerSwitcher), and the
// per-row Rename / Remove actions now drive shadcn Dialog +
// AlertDialog directly. This spec pins the "no native dialogs" rule:
// triggering Rename or Remove from a switcher row never falls back to
// window.* prompts.
test.describe("server-switcher row actions use themed dialogs", () => {
    test("Rename opens a themed dialog, not window.prompt", async ({ page }) => {
        const nativeDialogs: string[] = [];
        page.on("dialog", (d) => {
            nativeDialogs.push(`${d.type()}: ${d.message()}`);
            void d.dismiss();
        });

        await loginAsAdmin(page);
        // The sidebar collapses to an icon-only rail by default; the
        // server-switcher trigger is hidden in that mode. Expand it
        // before clicking the trigger.
        await page.getByRole("button", { name: /Expand sidebar/i }).click();
        await page.getByTestId("server-switcher-trigger").click();
        const row = page.getByTestId("server-row-0");
        await row.hover();
        await row.getByRole("button", { name: "Rename" }).click();

        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();
        await expect(dialog.getByRole("textbox")).toBeVisible();

        expect(nativeDialogs).toEqual([]);
    });

    test("Remove opens a themed confirmation, not window.confirm", async ({
        page,
    }) => {
        const nativeDialogs: string[] = [];
        page.on("dialog", (d) => {
            nativeDialogs.push(`${d.type()}: ${d.message()}`);
            void d.dismiss();
        });

        await loginAsAdmin(page);
        // The sidebar collapses to an icon-only rail by default; the
        // server-switcher trigger is hidden in that mode. Expand it
        // before clicking the trigger.
        await page.getByRole("button", { name: /Expand sidebar/i }).click();
        await page.getByTestId("server-switcher-trigger").click();
        const row = page.getByTestId("server-row-0");
        await row.hover();
        await row.getByRole("button", { name: "Remove" }).click();

        const alert = page.getByRole("alertdialog");
        await expect(alert).toBeVisible();
        await expect(alert.getByRole("button", { name: /Cancel/i })).toBeVisible();

        expect(nativeDialogs).toEqual([]);
    });
});
