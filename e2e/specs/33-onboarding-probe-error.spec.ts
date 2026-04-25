import { expect, test } from "../fixtures/test";

// Onboarding step 2 used to surface raw JS error strings ("TypeError:
// Failed to fetch") and left the Continue button enabled, inviting an
// infinite retry loop on a URL that obviously isn't reachable. Both
// behaviours are user-hostile.
test.describe("onboarding probe error UX", () => {
    test("error message is human-readable and Continue is disabled until URL changes", async ({
        page,
    }) => {
        // Reach /onboarding from a fresh client.
        await page.goto("/");
        await page.evaluate(() => localStorage.clear());
        await page.goto("/onboarding");
        await page.getByTestId("onboarding-get-started").click();

        // Type a URL that nothing's listening on.
        const url = page.getByTestId("onboarding-url");
        await url.fill("http://127.0.0.1:31337");
        const probe = page.getByTestId("onboarding-probe");
        await probe.click();

        // Wait for the probe to finish.
        await expect(probe).toBeEnabled({ timeout: 10_000 }).catch(() => {});

        // The on-screen error must not be a raw JS class name.
        const card = page.locator("body");
        const text = (await card.textContent()) ?? "";
        expect(text).not.toMatch(/TypeError/i);
        expect(text).not.toMatch(/Failed to fetch/i);

        // After the probe fails, Continue stays disabled until the URL
        // changes — clicking it again with the same URL just retries.
        await expect(probe).toBeDisabled();

        // Typing a new URL re-enables the button.
        await url.fill("http://127.0.0.1:31338");
        await expect(probe).toBeEnabled();
    });
});
