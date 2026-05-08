import { test } from "@playwright/test";

import { loginAsAdmin } from "../../fixtures/auth";
import { caption, clearOverlays, highlight, pause } from "../../fixtures/demo";

// 04-terminal-persistence — the global TerminalDrawer keeps shells
// alive across route changes. Open one on a host, navigate elsewhere,
// the same shell is still streaming.
test("walk: terminal survives route changes", async ({ page }) => {
    await loginAsAdmin(page);
    await pause(page, 500);

    await page.getByRole("button", { name: /Default created/i }).click();
    await pause(page, 500);
    await page.getByRole("link", { name: /Hosts$/ }).click();
    await pause(page, 500);
    await page
        .getByTestId("fleet-panel-table")
        .locator("table tbody tr")
        .first()
        .click();
    await pause(page, 800);

    await caption(
        page,
        "Open a shell with the page-header button — it docks into the global drawer.",
        1500,
    );
    // Scope the "Open terminal" button to the page header so the
    // status-bar's terminals-pill (aria-label "N open terminal")
    // doesn't double-resolve once the drawer is up.
    const openBtn = page
        .getByTestId("shell-content-frame")
        .getByRole("button", { name: "Open terminal" });
    await highlight(page, openBtn);
    await openBtn.click();
    await pause(page, 1800);

    await caption(page, "xterm streams over a WebSocket to the agent.", 1300);
    await pause(page, 1000);

    await caption(
        page,
        "Now navigate away — to Activity — without closing the shell.",
        1500,
    );
    await page.getByRole("link", { name: /Activity$/ }).click();
    await pause(page, 1100);

    await caption(
        page,
        "The drawer is still mounted at the bottom; the shell is still streaming.",
        1500,
    );
    await pause(page, 800);

    await caption(page, "Settings — same story.", 1000);
    await page.getByRole("link", { name: /Settings$/ }).click();
    await pause(page, 1100);

    await caption(
        page,
        "This was the bug that motivated the lift: the terminal used to remount on every route change and drop scrollback.",
        1900,
    );
    await pause(page, 800);

    await caption(page, "Back to the host — no flicker.", 900);
    await page.goBack();
    await pause(page, 700);
    await clearOverlays(page);
    await pause(page, 400);
});
