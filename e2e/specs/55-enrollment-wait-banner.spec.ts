import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// B3: after generating an install command the user used to land on
// a static dialog with a curl one-liner and no follow-up. They'd
// run the command, come back, and stare at /fleet wondering whether
// anything was happening — the success path was unclear.
//
// The fix:
//   1. The "Done" button on the issued-install dialog now navigates
//      to /projects/<slug>/fleet?await=enroll instead of just
//      closing the dialog in place.
//   2. FleetPage mounts the EnrollmentWaitBanner whenever the URL
//      carries that query param. The banner polls listHosts and
//      switches between three states: waiting (info, spinner) →
//      arrived (success, "open host" CTA) → stale (warning, after
//      90s with no new host).
//
// We test:
//   - The banner does not render without the query param.
//   - Visiting /fleet?await=enroll mounts the banner with the
//     "Waiting for an agent to enroll" copy and the info tone.
//   - Dismissing it strips the query param and the banner unmounts.
test.describe("enrollment wait banner", () => {
    test("absent without ?await=enroll", async ({ page }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/hosts");
        await expect(
            page.getByTestId("enrollment-wait-banner"),
        ).toHaveCount(0);
    });

    test("appears with ?await=enroll, dismiss strips the param", async ({ page }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/hosts?await=enroll");

        const banner = page.getByTestId("enrollment-wait-banner");
        await expect(banner).toBeVisible({ timeout: 10_000 });

        // Initial tone is info (still waiting). The component records
        // the tone via data-tone for stable querying without coupling
        // to colour values.
        await expect(banner).toHaveAttribute("data-tone", /info|success|warning/);
        const text = (await banner.textContent()) ?? "";
        expect(text).toMatch(/waiting for an agent to enroll/i);

        // Dismiss removes the param + unmounts the banner.
        await banner.getByRole("button", { name: /dismiss/i }).click();
        await expect(banner).toHaveCount(0);
        await expect(page).toHaveURL(/\/projects\/default\/hosts$/);
    });
});
