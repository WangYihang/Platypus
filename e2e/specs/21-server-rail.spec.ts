import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";
import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../fixtures/env";

// Slack-style server rail behaviour. Two profiles can point at the
// same backend URL — ServerProfile ids are per-profile, so adding
// "Mirror" against the same server is legitimate.
test.describe("server rail", () => {
    test("add second server, switch via click and keyboard, remove", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects/);
        await expect(page.getByTestId("server-tile-0")).toBeVisible();

        // Rail starts with exactly one tile.
        await expect(page.getByTestId("server-tile-1")).toHaveCount(0);

        // Open AddServerDialog and register a second profile.
        await page.getByTestId("server-rail-add").click();
        await page.getByTestId("add-server-url").fill(backendURL);
        await page.getByTestId("add-server-name").fill("Mirror");
        await page.getByRole("button", { name: "Continue" }).click();
        await page.getByTestId("add-server-username").fill(ADMIN_USERNAME);
        await page.getByTestId("add-server-password").fill(ADMIN_PASSWORD);
        await page.getByRole("button", { name: /^Log in$/ }).click();

        // Two tiles; Mirror is the newcomer and becomes active.
        await expect(page.getByTestId("server-tile-1")).toBeVisible({
            timeout: 15_000,
        });
        await expect(page.getByTestId("server-tile-1")).toHaveAttribute(
            "data-active",
            "true",
        );

        // Ctrl+1 flips to the first tile; Ctrl+2 flips back.
        await page.keyboard.press("Control+1");
        await expect(page.getByTestId("server-tile-0")).toHaveAttribute(
            "data-active",
            "true",
        );
        await page.keyboard.press("Control+2");
        await expect(page.getByTestId("server-tile-1")).toHaveAttribute(
            "data-active",
            "true",
        );

        // Remove the second profile via the rail's right-click menu.
        await page.getByTestId("server-tile-1").click({ button: "right" });
        await page.getByRole("menuitem", { name: /Remove/ }).click();
        // ContextMenuWrapper now uses an AlertDialog instead of
        // window.confirm — confirm via the dialog's Remove action.
        await page
            .getByRole("alertdialog")
            .getByRole("button", { name: /^Remove$/ })
            .click();
        // Rail is back to one tile; active flips to the remaining
        // profile as removeServer reassigns the active pointer.
        await expect(page.getByTestId("server-tile-1")).toHaveCount(0);
        await expect(page.getByTestId("server-tile-0")).toBeVisible();
        await expect(page.getByTestId("server-tile-0")).toHaveAttribute(
            "data-active",
            "true",
        );
    });
});
