import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// Newly-created projects should land the user inside the project
// they just made — they're about to do something in it. The previous
// behaviour kept them on /projects, where they had to click their
// own freshly-made tile to actually enter. The drop-back to the grid
// kills momentum at the only moment in the onboarding chain where
// the user has done a correct thing on their own.
//
// We navigate to /fleet (not /overview) because Fleet is the next
// concrete thing they should do (enrol an agent); /overview is a
// dashboard that's empty until at least one agent exists.
test.describe("project creation — navigates into the new project", () => {
    test("after Create succeeds the user lands on /projects/<slug>/fleet", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects");

        // The "New project" affordance lives inside the project
        // switcher dropdown (the trigger sits at the top of the
        // ProjectSidebar). Open the switcher, then click the footer
        // action.
        const sidebar = page.getByRole("complementary").nth(1);
        // The first button inside the sidebar is the switcher trigger.
        await sidebar.getByRole("button").first().click();
        await page
            .getByRole("button", { name: /new project/i })
            .first()
            .click();

        const dialog = page.getByRole("dialog");
        await expect(dialog).toBeVisible();

        // Slug is unique per run so we can re-run the spec without
        // colliding with the seeded "default" / "staging".
        const slug = `nav-test-${Date.now().toString(36)}`;
        await dialog.getByLabel(/name/i).fill(`Nav Test ${slug}`);
        await dialog.getByLabel(/slug/i).fill(slug);
        await dialog.getByRole("button", { name: /^create$/i }).click();

        // After creation we should be inside the new project, on Fleet.
        await expect(page).toHaveURL(
            new RegExp(`/projects/${slug}/fleet(?:[?#]|$)`),
            { timeout: 10_000 },
        );
    });
});
