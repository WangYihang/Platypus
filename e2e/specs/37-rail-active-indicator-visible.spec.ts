import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The active indicator bar in ServerRail was positioned with
// `left: -12` relative to the active tile, which placed it inside
// the parent's `overflow: auto` scroll region — clipping the bar
// off-screen on most viewports. Asserts the bar's bounding rect
// is fully inside the rail's bounding rect (i.e. nothing is being
// chopped off by an ancestor's overflow).
test.describe("rail active indicator", () => {
    test("active indicator bar is fully visible inside the rail", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects/);

        const rail = page.getByTestId("server-rail");
        const indicator = page.getByTestId("rail-active-indicator");
        await expect(rail).toBeVisible();
        await expect(indicator).toBeVisible();

        const railBox = await rail.boundingBox();
        const indBox = await indicator.boundingBox();
        if (!railBox || !indBox)
            throw new Error("rail or indicator has no bounding box");

        // The bar must be inside the rail: nothing leaked past the
        // left edge, fully contained vertically.
        expect(indBox.x).toBeGreaterThanOrEqual(railBox.x - 0.01);
        expect(indBox.x + indBox.width).toBeLessThanOrEqual(
            railBox.x + railBox.width + 0.01,
        );
        expect(indBox.y).toBeGreaterThanOrEqual(railBox.y - 0.01);
        expect(indBox.y + indBox.height).toBeLessThanOrEqual(
            railBox.y + railBox.height + 0.01,
        );
        // And it must actually have width / height (not 0 because of clip).
        expect(indBox.width).toBeGreaterThan(0);
        expect(indBox.height).toBeGreaterThan(0);
    });
});
