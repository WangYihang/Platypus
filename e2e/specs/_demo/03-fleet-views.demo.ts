import { test } from "@playwright/test";

import { loginAsAdmin } from "../../fixtures/auth";
import { caption, clearOverlays, highlight, pause } from "../../fixtures/demo";

// 03-fleet-views — Fleet's three views (Table / Timeline / Graph)
// share state via the Toggle group. Switching is one click; URL
// reflects ?view= so views are shareable.
//
// 2026-05 IA pass: the three-view toggle was retired — the legacy
// Fleet split into Hosts (table/cards), Activity → Sessions
// (timeline), and Hosts → Topology (graph) as separate routes. The
// narrative no longer matches the UI; skip until we re-record the
// demo around the new IA.
test.skip("walk: Table → Timeline → Graph in the Fleet view", async ({ page }) => {
    await loginAsAdmin(page);
    await pause(page, 500);

    await page.getByRole("button", { name: /Default created/i }).click();
    await pause(page, 600);

    await caption(page, "Fleet replaces the old Hosts / Sessions / Topology pages.", 1300);
    await highlight(page, page.getByRole("link", { name: /Hosts$/ }));
    await page.getByRole("link", { name: /Hosts$/ }).click();
    await pause(page, 800);

    await caption(page, "Default view: Table — the inventory.", 1100);
    await pause(page, 800);

    await caption(page, "Toggle to Timeline for live + historical sessions.", 1100);
    await highlight(page, page.getByRole("radio", { name: /Timeline/ }));
    await page.getByRole("radio", { name: /Timeline/ }).click();
    await pause(page, 1100);

    await caption(page, "Live and All filter chips work the same as before.", 1300);
    await pause(page, 700);

    await caption(page, "Graph view shows mesh links between agents.", 1100);
    await highlight(page, page.getByRole("radio", { name: /Graph/ }));
    await page.getByRole("radio", { name: /Graph/ }).click();
    await pause(page, 1500);

    await caption(
        page,
        "URL carries ?view=graph so the choice is shareable / refresh-stable.",
        1400,
    );
    await pause(page, 800);

    await caption(page, "Back to Table.", 900);
    await page.getByRole("radio", { name: /Table/ }).click();
    await pause(page, 700);

    await caption(page, "Click any host row to open its detail page.", 1200);
    await page
        .getByTestId("fleet-panel-table")
        .locator("table tbody tr")
        .first()
        .click();
    await pause(page, 1200);

    await caption(page, "Per-host: Info / Files / Sessions / Processes.", 1300);
    await pause(page, 800);
    await clearOverlays(page);
    await pause(page, 300);
});
