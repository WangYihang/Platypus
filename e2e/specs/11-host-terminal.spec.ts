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
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await page
            .getByTestId("fleet-panel-table")
            .locator("table tbody tr")
            .first()
            .click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/info$/);

        // Click the "Open terminal" action in the page header; that
        // pushes a shell into the global drawer and unhides it.
        await page.getByRole("button", { name: /Open terminal/i }).click();

        // The Terminal input is the focusable proxy xterm renders for
        // accessibility — its presence proves the WS upgrade landed.
        await expect(page.getByLabel("Terminal input")).toBeVisible({ timeout: 15_000 });
        // No "failed to open" / "ws failed" banner — that's the exact
        // symptom of the agent_id / session.id mix-up.
        await expect(page.getByText(/failed to open|ws failed/i)).not.toBeVisible();

        // Drawer stays mounted after we navigate away — this is the
        // regression that motivated lifting the terminal out of the
        // HostView tab strip.
        await page.getByRole("link", { name: /Overview$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview$/);
        await expect(page.getByLabel("Terminal input")).toBeVisible();

        await page.screenshot({
            path: shotPath("16-host-terminal.png"),
            fullPage: false,
        });
    });
});
