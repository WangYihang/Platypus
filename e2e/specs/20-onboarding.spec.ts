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

        // Land on the projects grid with the server switcher trigger
        // visible. Open it and confirm one row, labelled "Primary",
        // shows up as active. The sidebar collapses to an icon-only
        // rail by default in 2026-04+, hiding the switcher trigger;
        // expand it once before the assertion.
        await expect(page).toHaveURL(/\/projects$/, { timeout: 15_000 });
        await page.getByRole("button", { name: /Expand sidebar/i }).click();
        const trigger = page.getByTestId("server-switcher-trigger");
        await expect(trigger).toBeVisible();
        await trigger.click();
        const row = page.getByTestId("server-row-0");
        await expect(row).toHaveAttribute("data-active", "true");
        await expect(row).toContainText("Primary");
    });
});
