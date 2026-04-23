import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host tunnels", () => {
    test("Tunnels tab renders empty state + the New tunnel modal", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // Wait for xterm + tabs to settle (StrictMode double-mounts xterm).
        await expect(page.locator(".xterm-rows")).toBeVisible({ timeout: 15_000 });
        await page.waitForTimeout(800);
        await page.getByRole("tab", { name: "Tunnels" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/tunnels$/);

        // Empty state with the CTA visible.
        await expect(page.getByText("No active tunnels")).toBeVisible({
            timeout: 10_000,
        });
        await expect(
            page.getByRole("button", { name: /New tunnel/ }),
        ).toBeVisible();

        // Open the create modal — defaults to dynamic mode.
        await page.getByRole("button", { name: /New tunnel/ }).click();
        await expect(page.getByRole("dialog", { name: "New tunnel" })).toBeVisible();
        // Wait for modal animation.
        await page.waitForTimeout(1000);
        await expect(page.getByText(/Agent runs a SOCKS5 server/)).toBeVisible();

        await page.screenshot({
            path: shotPath("18-host-tunnels.png"),
            fullPage: false,
        });
    });
});
