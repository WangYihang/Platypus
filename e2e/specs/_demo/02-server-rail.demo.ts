import { test } from "@playwright/test";

import { loginAsAdmin } from "../../fixtures/auth";
import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../../fixtures/env";
import { caption, clearOverlays, highlight, pause } from "../../fixtures/demo";

// 02-server-switcher — the project switcher / server switcher folded
// the standalone Slack-style rail into a sidebar dropdown in the
// 2026-04 IA pass. Add a second profile, switch with the keyboard,
// rename via the themed dialog.
test("walk: switch between two servers from the rail", async ({ page }) => {
    await loginAsAdmin(page);
    await pause(page, 600);

    // The sidebar collapses to an icon-only rail by default. Expand
    // it once so the ServerSwitcher trigger renders inline (Cmd-K is
    // the surface for collapsed-rail switching, but the demo narrates
    // the visual switcher).
    await page.getByRole("button", { name: /Expand sidebar/i }).click();
    await pause(page, 400);

    await caption(
        page,
        "The rail shows one server right now — let's add a second.",
        1200,
    );
    await page.getByTestId("server-switcher-trigger").click();
    await pause(page, 500);
    await highlight(page, page.getByTestId("server-switcher-add"));
    await page.getByTestId("server-switcher-add").click();
    await pause(page, 700);

    await caption(page, "Same server URL, different display name.", 1000);
    const dlgUrl = page.getByTestId("add-server-url");
    await dlgUrl.click();
    await dlgUrl.fill("");
    await dlgUrl.pressSequentially(backendURL, { delay: 25 });
    await page.getByTestId("add-server-name").click();
    await page.getByTestId("add-server-name").pressSequentially("Mirror", {
        delay: 30,
    });
    await pause(page, 400);
    await page.getByRole("button", { name: "Continue" }).click();
    await pause(page, 700);

    await page.getByTestId("add-server-username").click();
    await page.getByTestId("add-server-username").pressSequentially(ADMIN_USERNAME, {
        delay: 30,
    });
    await page.getByTestId("add-server-password").click();
    await page.getByTestId("add-server-password").pressSequentially(ADMIN_PASSWORD, {
        delay: 30,
    });
    await pause(page, 300);
    await page.getByRole("button", { name: /^Log in$/ }).click();
    await pause(page, 1300);

    await caption(
        page,
        "Mirror is now active. Each row has a first-letter avatar with a hashed background.",
        1500,
    );
    await pause(page, 700);

    await caption(page, "Ctrl+1 jumps back to the first server.", 1100);
    await page.keyboard.press("Control+1");
    await pause(page, 900);

    await caption(page, "Ctrl+2 returns to Mirror — no extra clicks.", 1100);
    await page.keyboard.press("Control+2");
    await pause(page, 900);

    await caption(
        page,
        "Open the switcher again to Rename / Remove a row — themed dialogs, no native popups.",
        1500,
    );
    await page.getByTestId("server-switcher-trigger").click();
    await pause(page, 500);
    const row1 = page.getByTestId("server-row-1");
    await row1.hover();
    await highlight(page, row1.getByRole("button", { name: "Rename" }));
    await row1.getByRole("button", { name: "Rename" }).click();
    await pause(page, 700);

    const renameInput = page.getByRole("dialog").getByRole("textbox");
    await renameInput.click();
    await renameInput.fill("");
    await renameInput.pressSequentially("Production-clone", { delay: 30 });
    await pause(page, 400);
    await page.getByRole("dialog").getByRole("button", { name: /Save/ }).click();
    await pause(page, 1000);

    await caption(page, "Done — row is renamed.", 1200);
    await pause(page, 600);
    await clearOverlays(page);
    await pause(page, 300);
});
