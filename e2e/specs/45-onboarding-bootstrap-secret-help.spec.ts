import { expect, test } from "../fixtures/test";

import { backendURL } from "../fixtures/env";

// The Onboarding wizard's "Create the first admin" step asks for a
// "Server secret". Pre-fix copy:
//
//   "Paste the bootstrap secret printed on server startup and pick
//    admin credentials."
//
// That instruction was retired by the security audit (M2): the
// bootstrap secret no longer appears in stdout, it lives at
// <data-dir>/bootstrap.secret with mode 0600. A new admin who
// follows the wizard literally and runs `docker compose logs
// platypus-server` finds nothing and concludes the server is
// broken — the worst possible first impression.
//
// Lock the wizard's bootstrap step to point users at the file (and
// the canonical docker command) instead of stdout. We force the step
// to render by intercepting the public-info probe with
// admin_bootstrapped:false so the wizard takes the bootstrap branch
// regardless of what the live e2e backend has on it.
test.describe("onboarding — bootstrap secret help points at the file", () => {
    test("Bootstrap step copy mentions bootstrap.secret, not stdout", async ({
        page,
    }) => {
        // Stub the public probe so the wizard always thinks no admin
        // exists and renders the Bootstrap step. /api/v1/auth/info is
        // served on the SAME origin as the SPA in dev (Vite proxies),
        // but the SPA fetches it through the user-typed backendURL —
        // intercept that path.
        await page.route(/\/api\/v1\/auth\/info$/, (route) =>
            route.fulfill({
                status: 200,
                contentType: "application/json",
                body: JSON.stringify({
                    product: "platypus",
                    admin_bootstrapped: false,
                }),
            }),
        );

        await page.goto("/");
        await page.evaluate(() => localStorage.clear());
        await page.goto("/onboarding");

        // Welcome step should not carry the obsolete "printed at
        // startup" guidance either; check it before stepping past.
        const welcomeBody = (await page.locator("body").textContent()) ?? "";
        expect(welcomeBody, "welcome step must not promise a stdout secret").not.toMatch(
            /printed at\s*startup/i,
        );

        await page.getByTestId("onboarding-get-started").click();
        await page.getByTestId("onboarding-url").fill(backendURL);
        await page.getByTestId("onboarding-probe").click();

        // The bootstrap step renders the secret input.
        const secret = page.getByTestId("onboarding-secret");
        await expect(secret).toBeVisible({ timeout: 10_000 });

        const body = (await page.locator("body").textContent()) ?? "";
        // Forbid the obsolete copy regardless of whether it's in the
        // welcome paragraph or the bootstrap step.
        expect(body, "no stdout-source guidance anywhere").not.toMatch(
            /printed on server startup/i,
        );
        expect(body, "must point at bootstrap.secret on disk").toMatch(/bootstrap\.secret/);
        // The canonical Docker compose command should be there for
        // operators on the recommended deploy.
        expect(body, "must mention the docker compose exec command").toMatch(
            /docker compose exec/i,
        );
    });
});
