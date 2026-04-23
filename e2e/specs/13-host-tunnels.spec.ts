import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host tunnels", () => {
    test("Tunnels tab handles new tunnel creation", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await page.locator("table tbody tr").first().click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // Wait for xterm + tabs to settle.
        await expect(page.locator(".xterm-rows")).toBeVisible({ timeout: 15_000 });
        await page.waitForTimeout(1000);
        await page.getByRole("tab", { name: "Tunnels" }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/tunnels$/);

        // Empty state with the CTA visible.
        await expect(page.getByText("No active tunnels")).toBeVisible({
            timeout: 10_000,
        });

        // Open the create modal — defaults to dynamic mode.
        await page.getByRole("button", { name: /New tunnel/ }).click();
        await expect(page.getByRole("dialog", { name: "New tunnel" })).toBeVisible();
        // Wait for modal animation.
        await page.waitForTimeout(1000);

        // Create a tunnel.
        await page.getByRole("button", { name: "Create" }).click();

        // Modal should close and the tunnel should appear in the list.
        await expect(page.getByRole("dialog", { name: "New tunnel" })).not.toBeVisible();
        await page.waitForTimeout(3000);
        
        // A dynamic tunnel shows "socks5" and its local bind port.
        await expect(page.getByText("socks5", { exact: true })).toBeVisible({ timeout: 10_000 });

        await page.screenshot({
            path: shotPath("18-host-tunnels.png"),
            fullPage: false,
        });
    });
});
