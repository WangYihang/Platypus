import { Page } from "@playwright/test";

import { expect, test } from "../fixtures/test";
import { spawnAgent } from "../fixtures/agent";
import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../fixtures/env";

// loginAsAdminAt: like fixtures/auth::loginAsAdmin but lets the caller
// point at any origin serving the UI. Used so this spec can hit BOTH
// the vite dev server (StrictMode active) AND the prod-build UI
// embedded in the platypus-server binary, to catch StrictMode-only
// vs prod-mode regressions independently.
async function loginAsAdminAt(page: Page, origin: string) {
    await page.goto(`${origin}/login`);
    await page.getByLabel("Server URL", { exact: true }).fill(backendURL);
    await page.getByLabel("Username", { exact: true }).fill(ADMIN_USERNAME);
    await page.getByLabel("Password", { exact: true }).fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    await expect(page).toHaveURL(/\/projects/, { timeout: 15_000 });
}

// Recording playback regression guard.
//
// The user-visible bug it pins: after a session is recorded, opening
// the Audit > Recordings preview rendered the asciinema-player chrome
// fine but clicking the big "Play" overlay did nothing — no errors,
// no playback, the timer stayed at "--:--". Root cause: PreviewOverlay
// wrapped the player in a modal whose inner content set
// `onClick={(e) => e.stopPropagation()}` to prevent the backdrop's
// onClose from firing on inside-clicks. asciinema-player (built on
// Solid.js) attaches click handlers via document-level event
// delegation (`delegateEvents()` in
// node_modules/asciinema-player/dist/opts-*.js); stopping propagation
// killed the click bubble before it reached document, so the player
// never saw the click. Fix: gate the backdrop's onClose on
// `e.target === e.currentTarget` instead, which lets descendants'
// clicks bubble to document normally.
//
// What "playback worked" means here:
//   1. The .ap-overlay-start play overlay is removed from the DOM
//      after the click — v3.x removes the overlay element when the
//      driver transitions out of the uninitialised state.
//   2. The control-bar timer (.ap-time-elapsed) advances past the
//      initial "--:--" — getDuration / getCurrentTime hooks resolve
//      only after a successful driver init.
//   3. No console errors / page errors fire (handled by the suite-
//      wide quiet-console fixture).
//
// We exercise BOTH the dev (vite) and the prod (embedded) UI surfaces
// because StrictMode-only regressions and prod-only regressions have
// hit this code path before. The user reproduced the bug in prod, so
// the prod variant is the one that matters most — the dev variant is
// kept as a forward-looking guard against StrictMode drift.
const ORIGINS = [
    { label: "dev (vite)", origin: process.env.E2E_DEV_BASE ?? "http://127.0.0.1:5173" },
    { label: "prod (embedded)", origin: process.env.E2E_PROD_BASE ?? backendURL },
];

test.describe("recording playback", () => {
    for (const { label, origin } of ORIGINS) {
        test(`[${label}] clicking the play overlay starts playback`, async ({ page }) => {
            // Spin up an agent in the default project so the operator
            // has something to record against. The fixture handles
            // PAT minting + project-CA injection + waitForHost.
            const projectID = JSON.parse(
                process.env.PLATYPUS_E2E_PROJECTS || "[]",
            ).find((p: { slug: string }) => p.slug === "default")?.id as string;
            expect(projectID, "default project id").toBeTruthy();
            const agent = await spawnAgent({ projectID, labelForLogs: "rec-playback" });

            try {
                // Drive the UI: log in, open the Fleet host row, hit
                // "Open terminal" — that mounts xterm in the global
                // drawer and the server-side recording manager opens
                // a .cast file.
                await loginAsAdminAt(page, origin);
                await page.getByRole("button", { name: /Default created/i }).click();
                await page.getByRole("link", { name: /Fleet$/ }).click();
                await page
                    .getByTestId("fleet-panel-table")
                    .locator("table tbody tr")
                    .first()
                    .click();
                await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);
                await expect(page.getByText(/^\d+ active · /).first()).toContainText(
                    /^[1-9]\d* active/,
                    { timeout: 15_000 },
                );
                await page
                    .getByTestId("shell-content-frame")
                    .getByRole("button", { name: "Open terminal" })
                    .click();
                await expect(page.locator(".xterm-screen").first()).toBeAttached({
                    timeout: 15_000,
                });

                // Type a command into xterm and let the agent echo it
                // back. The xterm canvas is keyboard-focused once
                // mounted; just type into the page.
                await page.locator(".xterm-helper-textarea").first().focus();
                await page.keyboard.type("echo platypus-rec-marker\n");
                await page.waitForTimeout(800);

                // Close the terminal so the recording is finalised on
                // the server side. The drawer "Close" button finalises
                // the WS, which triggers Session.Finish.
                const closeBtn = page.getByRole("button", {
                    name: /Close terminal|Hide terminal/,
                });
                if (await closeBtn.count()) await closeBtn.first().click();
                await page.waitForTimeout(1500);

                // Open Audit > Recordings, find the just-completed
                // recording, click Preview.
                await page.goto(`${origin}/projects/default/audit/recordings`);
                await expect(page.getByRole("tab", { name: /Recordings/ })).toBeVisible();
                const previewBtn = page.getByRole("button", { name: /Preview/ }).first();
                await expect(previewBtn).toBeVisible({ timeout: 15_000 });
                await previewBtn.click();

                // Wait for the player chrome.
                await expect(page.locator(".ap-overlay-start")).toBeVisible({
                    timeout: 10_000,
                });
                await expect(page.locator(".ap-playback-button")).toBeAttached();

                // Pre-click sanity: timer is "--:--".
                expect(
                    (await page.locator(".ap-time-elapsed").first().textContent())?.trim(),
                ).toBe("--:--");

                // Click the big start overlay. v3 keeps
                // `.ap-overlay-start` in the DOM until the driver has
                // produced its first real frame; once the click
                // resolves successfully the overlay is removed.
                await page.locator(".ap-overlay-start").click();

                // Assert playback actually started:
                //   a. .ap-overlay-start gets removed.
                //   b. The timer advances off "--:--".
                await expect(page.locator(".ap-overlay-start")).toHaveCount(0, {
                    timeout: 5_000,
                });
                await expect
                    .poll(
                        async () =>
                            (await page
                                .locator(".ap-time-elapsed")
                                .first()
                                .textContent())?.trim(),
                        { timeout: 5_000 },
                    )
                    .not.toBe("--:--");
            } finally {
                await agent.kill();
            }
        });
    }
});
