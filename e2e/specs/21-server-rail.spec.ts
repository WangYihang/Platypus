import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";
import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../fixtures/env";

// Server-switcher behaviour (was the standalone Slack-style server
// rail before the 2026-04 IA pass; the rail collapsed into a dropdown
// at the top of the sidebar — see layout/ServerSwitcher.tsx). Two
// profiles can point at the same backend URL — ServerProfile ids are
// per-profile, so adding "Mirror" against the same server is
// legitimate. The spec exercises the same intent the rail spec did:
// add a profile, switch via click + keyboard, remove via the per-row
// trash button.
test.describe("server switcher", () => {
    test("add second server, switch via click and keyboard, remove", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects/);
        // Switcher trigger renders the active server name; opening it
        // surfaces the per-profile rows.
        const trigger = page.getByTestId("server-switcher-trigger");
        await expect(trigger).toBeVisible();
        await trigger.click();
        await expect(page.getByTestId("server-row-0")).toBeVisible();
        // Switcher starts with exactly one row.
        await expect(page.getByTestId("server-row-1")).toHaveCount(0);

        // "+ Add server" item opens AddServerDialog.
        await page.getByTestId("server-switcher-add").click();
        await page.getByTestId("add-server-url").fill(backendURL);
        await page.getByTestId("add-server-name").fill("Mirror");
        await page.getByRole("button", { name: "Continue" }).click();
        await page.getByTestId("add-server-username").fill(ADMIN_USERNAME);
        await page.getByTestId("add-server-password").fill(ADMIN_PASSWORD);
        await page.getByRole("button", { name: /^Log in$/ }).click();

        // Two rows now exist; Mirror is the newcomer and becomes active.
        await trigger.click();
        await expect(page.getByTestId("server-row-1")).toBeVisible({
            timeout: 15_000,
        });
        await expect(page.getByTestId("server-row-1")).toHaveAttribute(
            "data-active",
            "true",
        );

        // Close the dropdown (focus returns elsewhere) so keyboard
        // shortcuts hit the document-level handler in ProjectShell.
        await page.keyboard.press("Escape");

        // Ctrl+1 flips to the first row; Ctrl+2 flips back. The hotkey
        // is still wired in ProjectShell.useServerSwitchHotkeys and
        // operates on listServers() — switcher visibility is irrelevant.
        await page.keyboard.press("Control+1");
        await trigger.click();
        await expect(page.getByTestId("server-row-0")).toHaveAttribute(
            "data-active",
            "true",
        );
        await page.keyboard.press("Escape");
        await page.keyboard.press("Control+2");
        await trigger.click();
        await expect(page.getByTestId("server-row-1")).toHaveAttribute(
            "data-active",
            "true",
        );

        // Remove the second profile via the per-row Remove button.
        // The button surfaces on hover/focus — Playwright's hover()
        // makes the action group visible.
        const row1 = page.getByTestId("server-row-1");
        await row1.hover();
        await row1.getByRole("button", { name: "Remove" }).click();
        await page
            .getByRole("alertdialog")
            .getByRole("button", { name: /^Remove$/ })
            .click();

        // Switcher is back to one row; active flips to the remaining
        // profile as removeServer reassigns the active pointer.
        await trigger.click();
        await expect(page.getByTestId("server-row-1")).toHaveCount(0);
        await expect(page.getByTestId("server-row-0")).toBeVisible();
        await expect(page.getByTestId("server-row-0")).toHaveAttribute(
            "data-active",
            "true",
        );
    });
});
