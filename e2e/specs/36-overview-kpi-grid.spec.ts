import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The Overview's KPI strip had four MetricCards: Hosts / Online now /
// Ingress / Live sessions. Three are honest counts (1/1/1); the
// Ingress one stuffed a host:port URL into the same big-number slot
// and broke the visual grid — the URL shrank to ~half the size of
// the actual numbers and threw the rhythm off in every screenshot.
//
// Move Ingress out of the KPI grid (the existing dedicated "Ingress"
// card below the chart row already presents the address). Asserts
// the KPI strip has 3 numeric cards, none of them labelled
// "Ingress".
test.describe("project overview KPI strip", () => {
    test("KPI grid has 3 cards and Ingress is not one of them", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview/);

        const tiles = page.getByTestId("metric-card");
        await expect(tiles).toHaveCount(3);

        for (let i = 0; i < 3; i++) {
            const text = (await tiles.nth(i).textContent()) ?? "";
            expect(text.toLowerCase()).not.toContain("ingress");
        }
    });
});
