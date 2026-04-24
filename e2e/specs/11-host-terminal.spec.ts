import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host terminal", () => {
    // The baseline agent globalSetup spawns is enrolled and link-
    // connected by the time this spec runs. HostView resolves the
    // pickedSessionID to host.agent_id (NOT session.id) — using the
    // session row's UUID 404'd against /api/v1/terminal/:id/ws and
    // /api/v1/agents/:id/fs because the registry is keyed on the
    // cert's URI SAN. This spec exists to keep that wiring honest:
    // a regression to "session UUID as the route param" would surface
    // here as the xterm prompt never appearing.
    test("Terminal tab mounts xterm and lands on the agent's prompt", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        await expect(page.getByRole("tab", { name: "Terminal", selected: true })).toBeVisible();
        // The Terminal input is the focusable proxy xterm renders for
        // accessibility — its presence proves the WS upgrade landed.
        await expect(page.getByLabel("Terminal input")).toBeVisible({ timeout: 15_000 });
        // No "failed to open" / "ws failed" banner — that's the exact
        // symptom of the agent_id / session.id mix-up.
        await expect(page.getByText(/failed to open|ws failed/i)).not.toBeVisible();

        await page.screenshot({
            path: shotPath("16-host-terminal.png"),
            fullPage: false,
        });
    });
});
