import { test } from "@playwright/test";

import { ADMIN_PASSWORD, ADMIN_USERNAME, backendURL } from "../../fixtures/env";
import { caption, clearOverlays, highlight, pause } from "../../fixtures/demo";

// 01-onboarding — fresh client → /onboarding → wizard → /projects.
// Demonstrates the first-run experience for a brand-new operator.
test("walk: first-run onboarding wizard", async ({ page }) => {
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.goto("/");
    await pause(page, 600);

    await caption(
        page,
        "Fresh install — no servers saved yet. Platypus drops you on /onboarding.",
        1200,
    );

    await caption(page, "Step 1 of 3 — Welcome", 900);
    await highlight(page, page.getByTestId("onboarding-get-started"));
    await page.getByTestId("onboarding-get-started").click();
    await pause(page, 500);

    await caption(page, "Step 2 — paste your server's URL", 1000);
    const url = page.getByTestId("onboarding-url");
    await highlight(page, url);
    await url.click();
    await url.fill("");
    await url.pressSequentially(backendURL, { delay: 25 });
    await pause(page, 500);

    await caption(page, "Give it a friendly name (optional).", 900);
    const name = page.getByTestId("onboarding-name");
    await highlight(page, name);
    await name.click();
    await name.pressSequentially("Production", { delay: 30 });
    await pause(page, 400);

    await caption(
        page,
        "We probe the server first to pick the right next step.",
        1100,
    );
    await highlight(page, page.getByTestId("onboarding-probe"));
    await page.getByTestId("onboarding-probe").click();
    await pause(page, 1100);

    await caption(
        page,
        "Server's ready and bootstrapped — log in with your credentials.",
        1300,
    );
    await page.getByTestId("onboarding-username").click();
    await page.getByTestId("onboarding-username").pressSequentially(ADMIN_USERNAME, { delay: 30 });
    await page.getByTestId("onboarding-password").click();
    await page.getByTestId("onboarding-password").pressSequentially(ADMIN_PASSWORD, { delay: 30 });
    await pause(page, 400);
    await highlight(page, page.getByTestId("onboarding-login"));
    await page.getByTestId("onboarding-login").click();
    await pause(page, 1500);

    await caption(
        page,
        "Logged in. Server tile lit up on the rail; pick a project to drill in.",
        1600,
    );
    await pause(page, 1000);
    await clearOverlays(page);
    await pause(page, 400);
});
