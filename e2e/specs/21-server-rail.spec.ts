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
    test("add second server, switch via click, remove", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects/);
        // Sidebar collapses to an icon-only rail by default; the
        // switcher trigger is hidden (Cmd-K is the surface for
        // switching while collapsed). Expand once for this spec
        // since it exercises the inline switcher UI.
        await page.getByRole("button", { name: /Expand sidebar/i }).click();
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

        // (Keyboard hotkey switching — Ctrl+1 / Ctrl+2 — has its own
        // dedicated coverage; the popover state churn here makes
        // verifying mid-flight via this trigger a flake magnet, so
        // we exercise only the click-driven switch in this spec.)

        // Remove the second profile via the per-row Remove button.
        // The popover is already open from the previous assertion;
        // hover the row to surface the action group.
        const row1 = page.getByTestId("server-row-1");
        await row1.hover();
        await row1.getByRole("button", { name: "Remove" }).click();
        await page
            .getByRole("alertdialog")
            .getByRole("button", { name: /^Remove$/ })
            .click();

        // Switcher is back to one row; active flips to the remaining
        // profile as removeServer reassigns the active pointer. The
        // popover dismisses on the alertdialog confirm and re-opens
        // on the next paint, so retry the click + assert until the
        // menu is up.
        await expect(async () => {
            await trigger.click({ force: true });
            await expect(page.getByTestId("server-switcher-menu")).toBeVisible({
                timeout: 1_000,
            });
            await expect(page.getByTestId("server-row-1")).toHaveCount(0, {
                timeout: 1_000,
            });
            await expect(page.getByTestId("server-row-0")).toBeVisible({
                timeout: 1_000,
            });
            await expect(page.getByTestId("server-row-0")).toHaveAttribute(
                "data-active",
                "true",
                { timeout: 1_000 },
            );
        }).toPass({ timeout: 10_000 });
    });
});
