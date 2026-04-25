import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The ServerRail's right-click menu used window.prompt() to rename
// and window.confirm() to remove — both render the OS-native gray
// alert chrome, which clashes with the dark-themed app and feels
// alien on the desktop Wails build. Replace with shadcn Dialog +
// AlertDialog so the surface stays in-app and themable.
test.describe("rail context menu uses themed dialogs", () => {
    test("Rename opens a themed dialog, not window.prompt", async ({ page }) => {
        // Fail loudly if any native dialog appears. The rail's
        // context menu must drive its own UI.
        const nativeDialogs: string[] = [];
        page.on("dialog", (d) => {
            nativeDialogs.push(`${d.type()}: ${d.message()}`);
            void d.dismiss();
        });

        await loginAsAdmin(page);
        await page.getByTestId("server-tile-0").click({ button: "right" });
        await page.getByRole("menuitem", { name: /Rename/ }).click();

        // Themed rename dialog appears — has role=dialog and an input.
        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();
        await expect(dialog.getByRole("textbox")).toBeVisible();

        // No native dialog leaked.
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
        await page.getByTestId("server-tile-0").click({ button: "right" });
        await page.getByRole("menuitem", { name: /Remove/ }).click();

        const alert = page.getByRole("alertdialog");
        await expect(alert).toBeVisible();
        await expect(alert.getByRole("button", { name: /Cancel/i })).toBeVisible();

        expect(nativeDialogs).toEqual([]);
    });
});
