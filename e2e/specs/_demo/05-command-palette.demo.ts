import { test } from "@playwright/test";

import { loginAsAdmin } from "../../fixtures/auth";
import { caption, clearOverlays, highlight, pause } from "../../fixtures/demo";

// 05-command-palette — Cmd/Ctrl+K opens a global cmdk palette with
// fuzzy nav: jump between pages, switch project, open a shell on
// any host without touching the sidebar.
test("walk: Cmd/Ctrl+K command palette", async ({ page }) => {
    await loginAsAdmin(page);
    await pause(page, 500);

    await page.getByRole("button", { name: /Default created/i }).click();
    await pause(page, 600);

    await caption(
        page,
        "Cmd / Ctrl+K opens the command palette from anywhere in the app.",
        1500,
    );
    await page.keyboard.press("Control+k");
    await pause(page, 900);

    await caption(page, "Fuzzy match — type to filter.", 1100);
    await page.keyboard.type("fleet", { delay: 70 });
    await pause(page, 800);

    await caption(page, "Enter jumps you straight there.", 900);
    await page.keyboard.press("Enter");
    await pause(page, 1100);

    await caption(page, "Open it again to switch projects or hosts.", 1200);
    await page.keyboard.press("Control+k");
    await pause(page, 700);

    await caption(page, "Type a host alias to navigate to its detail page…", 1200);
    await page.keyboard.type("vm", { delay: 80 });
    await pause(page, 800);
    await page.keyboard.press("Enter");
    await pause(page, 1300);

    await caption(
        page,
        "…or pick the 'Open shell on …' entry to drop a terminal in the bottom drawer.",
        1700,
    );
    await page.keyboard.press("Control+k");
    await pause(page, 600);
    await page.keyboard.type("shell vm", { delay: 70 });
    await pause(page, 800);
    await page.keyboard.press("Enter");
    await pause(page, 1500);

    await caption(page, "Done. Keyboard-first, mouse optional.", 1300);
    await pause(page, 700);
    await clearOverlays(page);
    await pause(page, 300);
});
