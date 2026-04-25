import { expect, test } from "../fixtures/test";

import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../fixtures/env";

// The Toaster (Sonner) and StatusBar were both anchored to the
// bottom-right corner of the viewport. Sonner's default offset is
// 32px and the StatusBar is 28px tall — every toast covered the
// "Ingress · Sessions · Server" info row.
//
// Drives a real toast through the onboarding wizard's success path
// (`toast.success("Welcome to …")`) and asserts the toast's bounding
// rect doesn't overlap the StatusBar's bounding rect.
test.describe("toast / statusbar layout", () => {
    test("toast does not overlap StatusBar", async ({ page }) => {
        await page.goto("/");
        await page.evaluate(() => localStorage.clear());
        await page.goto("/onboarding");

        await page.getByTestId("onboarding-get-started").click();
        await page.getByTestId("onboarding-url").fill(backendURL);
        await page.getByTestId("onboarding-name").fill("Production");
        await page.getByTestId("onboarding-probe").click();
        await page.getByTestId("onboarding-username").fill(ADMIN_USERNAME);
        await page.getByTestId("onboarding-password").fill(ADMIN_PASSWORD);
        await page.getByTestId("onboarding-login").click();

        // Land on /projects with a "Welcome to Production" toast.
        await expect(page).toHaveURL(/\/projects$/, { timeout: 15_000 });

        const toast = page
            .locator("[data-sonner-toast], li[data-sonner-toast]")
            .first();
        await expect(toast).toBeVisible({ timeout: 5_000 });

        const sb = page.getByTestId("status-bar");
        await expect(sb).toBeVisible();

        const tb = await toast.boundingBox();
        const sbBox = await sb.boundingBox();
        if (!tb || !sbBox)
            throw new Error("toast or status bar has no bounding box");

        const overlap =
            tb.x < sbBox.x + sbBox.width &&
            tb.x + tb.width > sbBox.x &&
            tb.y < sbBox.y + sbBox.height &&
            tb.y + tb.height > sbBox.y;
        expect(overlap).toBe(false);

        // Require a visible breathing-room gap so toasts and status
        // info read as separate regions, not a single stacked
        // element. 8px matches the rest of our `space[2]` rhythm.
        const gap = sbBox.y - (tb.y + tb.height);
        expect(gap).toBeGreaterThanOrEqual(8);
    });
});
