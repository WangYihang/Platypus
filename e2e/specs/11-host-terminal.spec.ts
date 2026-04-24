import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host terminal", () => {
    // v2 doesn't surface connected agents as live "sessions" yet, and
    // TerminalTab won't mount xterm until a session exists (it renders
    // a "No live session — Waiting for the agent to reconnect to a
    // listener" empty state otherwise). We can only assert the tab
    // strip + placeholder are wired; expanding the test to actual
    // command execution needs the server to auto-create a session row
    // on agent-link connect, which is a follow-up on the v2 roadmap.
    test("Terminal tab routes to /terminal and shows empty-state when no session", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        await expect(page.getByRole("tab", { name: "Terminal", selected: true })).toBeVisible();
        await expect(page.getByText(/No live session/).first()).toBeVisible({ timeout: 10_000 });

        await page.screenshot({
            path: shotPath("16-host-terminal.png"),
            fullPage: false,
        });
    });
});
