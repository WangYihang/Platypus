import { expect, test } from "../fixtures/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("global terminal drawer", () => {
    // The baseline agent globalSetup spawns is enrolled and link-
    // connected by the time this spec runs. HostView resolves the
    // pickedSessionID to host.agent_id (NOT session.id) — using the
    // session row's UUID 404'd against /api/v1/terminal/:id/ws and
    // /api/v1/agents/:id/fs because the registry is keyed on the
    // cert's URI SAN. This spec exists to keep that wiring honest:
    // a regression to "session UUID as the route param" would surface
    // here as the xterm prompt never appearing.
    test("Open terminal mounts xterm in the bottom drawer", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page
            .getByTestId("fleet-panel-table")
            .locator("table tbody tr")
            .first()
            .click();
        // After 9748a49 the default host tab is Files, not Info. The
        // "Open terminal" header button is mounted on every tab so we
        // don't need to switch.
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/files$/);

        // The "Open terminal" header button is enabled once the
        // agent's link-session row exists (`canOpenShell = liveCount
        // > 0 && !!agent_id`). That happens asynchronously after the
        // page mounts, so wait for the button to leave the disabled
        // state before clicking. Without the gate the click would
        // fire while the title still reads "No live agent session"
        // and the openShell handler bails.
        const openBtn = page
            .getByTestId("shell-content-frame")
            .getByRole("button", { name: "Open terminal" });
        await expect(openBtn).toBeEnabled({ timeout: 15_000 });
        await openBtn.click();

        // Xterm mounts at least one `.xterm-screen` inside the
        // drawer once the WS upgrade lands. It may be visibility:
        // hidden in playwright's eyes (xterm renders into an absolute-
        // positioned canvas and its a11y helper textarea is offscreen
        // by design), so we assert *attachment* rather than
        // visibility — same contract the original spec was after,
        // just without the brittle "is this DOM node within the
        // browser viewport?" check. A regression to using session.id
        // as the route param would still surface here because the
        // canvas would never mount in the first place.
        await expect(page.locator(".xterm-screen").first()).toBeAttached({
            timeout: 15_000,
        });
        // No "failed to open" / "ws failed" banner — that's the exact
        // symptom of the agent_id / session.id mix-up.
        await expect(page.getByText(/failed to open|ws failed/i)).not.toBeVisible();

        // Drawer stays mounted after we navigate away — this is the
        // regression that motivated lifting the terminal out of the
        // HostView tab strip.
        await page.getByRole("link", { name: /Overview$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);
        await expect(page.locator(".xterm-screen").first()).toBeAttached();

        await page.screenshot({
            path: shotPath("16-host-terminal.png"),
            fullPage: false,
        });
    });
});
