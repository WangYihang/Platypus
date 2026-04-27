import { expect, test } from "../fixtures/test";

// The Onboarding wizard's server-URL field is a new admin's first
// concrete instruction. The placeholder was stuck on the legacy
// `http://127.0.0.1:7331` scheme + REST-only port from before the
// unified TLS ingress landed; copy-pasting it leads straight into a
// probe error ("This URL responded but doesn't look like a Platypus
// server") and burns trust on the first interaction.
//
// New users follow placeholders literally, so this is the canonical
// example to keep up to date with whatever the server actually serves
// today (https + the unified ingress port).
test.describe("onboarding URL placeholder", () => {
    test("placeholder uses https + the unified ingress port", async ({ page }) => {
        await page.goto("/");
        await page.evaluate(() => localStorage.clear());
        await page.goto("/onboarding");
        await page.getByTestId("onboarding-get-started").click();

        const url = page.getByTestId("onboarding-url");
        const placeholder = await url.getAttribute("placeholder");

        expect(placeholder, "placeholder should hint at the live ingress").not.toBeNull();
        // Must be https — http examples MITM-able, anti-pattern in a
        // security-sensitive product.
        expect(placeholder!).toMatch(/^https:\/\//);
        // The legacy 7331 / 13337 / 13339 ports are removed.
        expect(placeholder!).not.toMatch(/:7331\b/);
        expect(placeholder!).not.toMatch(/:13337\b/);
        expect(placeholder!).not.toMatch(/:13339\b/);
        // The unified ingress in docker-compose / Dockerfile is :9443.
        expect(placeholder!).toMatch(/:9443\b/);
    });
});
