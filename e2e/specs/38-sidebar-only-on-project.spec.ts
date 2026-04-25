import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// /projects is the project-tile grid. The ProjectSidebar there had
// nothing useful to show — its nav items all require an active
// project — so the 240px column rendered with just the brand and
// the "Pick a project…" placeholder, wasting horizontal space.
//
// Hide the ProjectSidebar on the /projects route; bring it back as
// soon as the user enters a project subroute.
test.describe("ProjectSidebar visibility", () => {
    test("hidden on /projects, visible on /projects/:slug/*", async ({ page }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects$/);
        await expect(page.getByTestId("project-sidebar")).toHaveCount(0);

        await page.getByRole("button", { name: /Default created/i }).click();
        await expect(page).toHaveURL(/\/projects\/default\/overview/);
        await expect(page.getByTestId("project-sidebar")).toBeVisible();
    });
});
