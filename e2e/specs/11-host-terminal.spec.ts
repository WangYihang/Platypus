import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host terminal", () => {
    test("terminal tab mounts xterm against the live session", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // xterm renders its prompt cells into `.xterm-rows`. Its presence
        // proves the full roundtrip: /ws/ticket → WS upgrade → xterm.open.
        const rows = page.locator(".xterm-rows");
        await expect(rows).toBeVisible({ timeout: 15_000 });

        // Let StrictMode's double-mount settle so the screenshot catches
        // the stable second instance rather than a half-torn-down first.
        await page.waitForTimeout(800);

        // There should not be a `[failed to open: ...]` error text in
        // the terminal (see Terminal.tsx line 117).
        await expect(rows).not.toContainText("failed to open", {
            timeout: 1_000,
        });

        await page.screenshot({
            path: shotPath("16-host-terminal.png"),
            fullPage: false,
        });
    });
});
