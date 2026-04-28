import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The active server is now marked inside the ServerSwitcher dropdown
// (was a 3 px Slack-style bar on the standalone rail) — the active
// row gets `bg-accent` plus `data-active="true"` and stays fully
// inside the dropdown menu's scroll container. This spec pins both:
//   1. The dropdown contains a row marked active.
//   2. The active marker (the `rail-active-indicator` testid is kept
//      on the row's name span for backwards-compatible coverage)
//      sits fully within the menu's bounding rect — no overflow clip.
test.describe("server-switcher active row", () => {
    test("active marker is fully visible inside the dropdown", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects/);

        await page.getByTestId("server-switcher-trigger").click();
        const menu = page.getByTestId("server-switcher-menu");
        const indicator = page.getByTestId("rail-active-indicator");
        await expect(menu).toBeVisible();
        await expect(indicator).toBeVisible();

        const menuBox = await menu.boundingBox();
        const indBox = await indicator.boundingBox();
        if (!menuBox || !indBox)
            throw new Error("menu or indicator has no bounding box");

        expect(indBox.x).toBeGreaterThanOrEqual(menuBox.x - 0.01);
        expect(indBox.x + indBox.width).toBeLessThanOrEqual(
            menuBox.x + menuBox.width + 0.01,
        );
        expect(indBox.y).toBeGreaterThanOrEqual(menuBox.y - 0.01);
        expect(indBox.y + indBox.height).toBeLessThanOrEqual(
            menuBox.y + menuBox.height + 0.01,
        );
        expect(indBox.width).toBeGreaterThan(0);
        expect(indBox.height).toBeGreaterThan(0);
    });
});
