import { expect } from "@playwright/test";

import { test } from "../fixtures/agent";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("multi-agent host", () => {
    // The `liveAgent` fixture spawns one extra platypus-agent against
    // the seeded listener for the duration of this spec. After the
    // spec, the fixture SIGTERMs it and waits for it to exit so the
    // baseline single-agent state is restored for later specs.
    test("two agents → two live sessions on the project", async ({
        page,
        liveAgent,
    }) => {
        // liveAgent already waited for the session count to grow by 1
        // before resolving; sanity-check it has a pid.
        expect(liveAgent.pid).toBeGreaterThan(0);

        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Sessions$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/sessions$/);

        // Live filter is on by default — at least 2 rows visible.
        const rows = page.locator("table tbody tr");
        await expect(rows).toHaveCount(2, { timeout: 10_000 });
        // Both rows show the live StatusPill.
        await expect(page.getByText("live", { exact: true })).toHaveCount(2);

        await page.screenshot({
            path: shotPath("20-multi-agent.png"),
            fullPage: false,
        });
    });
});
