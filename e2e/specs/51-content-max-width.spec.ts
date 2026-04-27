import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// P3: on wide displays the inner content area used to stretch edge-to-
// edge inside <main>. Overview KPI strips, Fleet tables, and the
// Members list all ran 1500+ pixels wide on 1920px monitors with
// individual rows of 100+ characters — readable, but eye-fatiguing.
//
// Cap the inner content at 1280px so text columns stay within roughly
// 80-100 characters across all surfaces. The rail / sidebar / status
// bar are unaffected; only the <main> body shrinks.
//
// Tested at a 1920px viewport. The cap is centered, so we check the
// rendered width of an inner content wrapper.
test.describe("content max-width on wide displays", () => {
    test.use({ viewport: { width: 1920, height: 900 } });

    test("inner content does not stretch edge-to-edge above ~1300px", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects");

        // The shell wraps the routed Outlet in a content frame tagged
        // with data-testid="shell-content-frame". We measure that
        // element's bounding box.
        const frame = page.getByTestId("shell-content-frame");
        await expect(frame).toBeVisible();
        const box = await frame.boundingBox();
        expect(box, "shell-content-frame has no bounding box").not.toBeNull();
        // 1280px cap + 1px slack for sub-pixel rounding. Anything
        // wider means the cap regressed.
        expect(box!.width).toBeLessThanOrEqual(1281);
        // Not too narrow either — at 1920px viewport with rail (56) +
        // sidebar (240) we expect at least ~1100 of usable width
        // before the cap kicks in if no cap; with the cap, it should
        // be near the 1280 bound.
        expect(box!.width).toBeGreaterThanOrEqual(800);
    });
});
