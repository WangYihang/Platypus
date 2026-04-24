import { expect, test } from "../fixtures/test";

import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../fixtures/env";

// Fresh localStorage → first-run wizard takes the user from "never
// opened the app" to a logged-in rail with one tile, without ever
// seeing the classic /login form.
test.describe("onboarding", () => {
    test("fresh client lands on /onboarding and completes the wizard", async ({
        page,
    }) => {
        // Clear every persisted key on the origin so the redirect
        // guard in RequireAuth picks /onboarding.
        await page.goto("/");
        await page.evaluate(() => localStorage.clear());
        await page.goto("/");
        await expect(page).toHaveURL(/\/onboarding$/);

        // Step 1: Welcome.
        await page
            .getByTestId("onboarding-get-started")
            .click();

        // Step 2: URL + probe. The backend is already running with
        // one admin (globalSetup), so the probe response routes us
        // to the Log-in branch.
        await page.getByTestId("onboarding-url").fill(backendURL);
        await page.getByTestId("onboarding-name").fill("Primary");
        await page.getByTestId("onboarding-probe").click();

        // Step 3b: log in with the seeded admin.
        await page
            .getByTestId("onboarding-username")
            .fill(ADMIN_USERNAME);
        await page
            .getByTestId("onboarding-password")
            .fill(ADMIN_PASSWORD);
        await page.getByTestId("onboarding-login").click();

        // Land on the projects grid with the rail visible and one
        // tile labelled "P" (first char of "Primary").
        await expect(page).toHaveURL(/\/projects$/, { timeout: 15_000 });
        const rail = page.getByTestId("server-rail");
        await expect(rail).toBeVisible();
        const tile = page.getByTestId("server-tile-0");
        await expect(tile).toHaveText(/^P/);
        await expect(tile).toHaveAttribute("data-active", "true");
    });
});
