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
                // Post-2466550 IA: "Fleet" was renamed to "Hosts"
                // and the URL moved from /fleet to /hosts. The
                // table testid kept its old `fleet-panel-table`
                // name even after the rename.
                await page.getByRole("link", { name: /^Hosts$/ }).click();
                await page
                    .getByTestId("fleet-panel-table")
                    .locator("table tbody tr")
                    .first()
                    .click();
                await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);
                // Post-2466550 IA: "Open terminal" is now an icon-
                // only button in the host header (aria-label
                // "Open terminal"). The button is enabled once the
                // agent's link session exists, which happens
                // immediately for our spawned agent.
                await page
                    .getByRole("button", { name: "Open terminal", exact: true })
                    .first()
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
                // recording, click Preview. Capture the cast bytes
                // off the wire so we can also assert no bogus
                // sub-floor resize events landed in the file (the
                // "playback suddenly shows huge text mid-stream"
                // regression — xterm-fit reading from a still-
                // animating drawer would emit r=9x7 frames).
                let castBytes: string | null = null;
                page.on("response", async (resp) => {
                    if (!resp.url().endsWith("/cast")) return;
                    try {
                        castBytes = (await resp.body()).toString("utf8");
                    } catch {}
                });
                // Post-2466550 IA: /audit/recordings → /activity/recordings.
                await page.goto(`${origin}/projects/default/activity/recordings`);
                await expect(page.getByRole("tab", { name: /Recordings/ })).toBeVisible();
                const previewBtn = page.getByRole("button", { name: /Preview/ }).first();
                await expect(previewBtn).toBeVisible({ timeout: 15_000 });

                // Thumbnail regression: each completed recording's
                // card mounts a chrome-less asciinema-player as a
                // thumbnail. Assert at least one thumbnail painted
                // a non-uniform canvas (i.e. the poster actually
                // replayed terminal content). Without the lazy-mount
                // working OR without the poster, the canvas would be
                // a solid color and this would fail. Only on the
                // prod variant — vite's dev StrictMode double-mount
                // makes canvas state racey.
                // Thumbnail regression: each completed recording's
                // card mounts a chrome-less asciinema-player as a
                // thumbnail. We just assert the thumbnail's canvas
                // is attached + visible — proves the
                // IntersectionObserver lazy-mount fired and the
                // player painted into the page. We deliberately do
                // NOT read pixels: asciinema-player's renderer
                // calls getImageData internally and Chrome's
                // willReadFrequently perf hint trips the suite's
                // quietConsole fixture if we add more reads on top.
                await expect(
                    page.locator(".recording-thumbnail .ap-term canvas").first(),
                ).toBeVisible({ timeout: 10_000 });

                await previewBtn.click();

                // Scope every player assertion to the preview modal.
                // Each card now mounts its own (chrome-less)
                // asciinema-player as a thumbnail, so a bare
                // `.ap-overlay-start` locator resolves to N+1
                // elements on this page.
                const preview = page.getByTestId("recording-preview");
                await expect(preview.locator(".ap-overlay-start")).toBeVisible({
                    timeout: 10_000,
                });
                await expect(preview.locator(".ap-playback-button")).toBeAttached();

                // Pre-click: the player's poster ("npt:0:0.5") forces
                // init() to run synchronously inside create(), so by
                // the time the chrome is mounted the timer label
                // already shows a real value (not the lazy-init
                // "--:--" placeholder). This pins both the black-
                // preview fix AND the "duration not shown" complaint.
                await expect
                    .poll(
                        async () =>
                            (
                                await preview.locator(".ap-time-elapsed").first().textContent()
                            )?.trim(),
                        { timeout: 5_000 },
                    )
                    .not.toBe("--:--");

                // Click the big start overlay. v3 keeps
                // `.ap-overlay-start` in the DOM until the driver has
                // produced its first real frame; once the click
                // resolves successfully the overlay is removed.
                await preview.locator(".ap-overlay-start").click();

                // Assert playback actually started:
                //   a. .ap-overlay-start gets removed.
                //   b. The timer advances off "--:--".
                await expect(preview.locator(".ap-overlay-start")).toHaveCount(0, {
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

                // Cast hygiene: no sub-floor resize events. Each
                // event line starts with `[t, "r", "<cols>x<rows>"]`
                // — match those and assert cols >= 40 and rows >= 10
                // (the floor enforced by Terminal.tsx +
                // parseResizeFrame). Without this guard, xterm-fit's
                // 9×7 transients during the drawer animation get
                // baked into the recording and replay as a sudden
                // "huge text" jump mid-stream.
                expect(castBytes, "captured /cast body").not.toBeNull();
                const resizeRe = /^\[\d+\.?\d*,\s*"r",\s*"(\d+)x(\d+)"\]/;
                for (const line of (castBytes as unknown as string).split("\n")) {
                    const m = resizeRe.exec(line);
                    if (!m) continue;
                    const cols = parseInt(m[1], 10);
                    const rows = parseInt(m[2], 10);
                    expect(
                        cols >= 40 && rows >= 10,
                        `[${label}] sub-floor resize event in cast: ${cols}x${rows} (line: ${line})`,
                    ).toBe(true);
                }
            } finally {
                await agent.kill();
            }
        });
    }
});
