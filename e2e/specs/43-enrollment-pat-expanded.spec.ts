import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// `PAT` was used 7+ times across EnrollmentPage with zero in-page
// expansion. New users have to guess what it stands for (Personal /
// Project / Provider Access Token?). Worse, the user-facing copy
// reaches into implementation jargon ("mints a PAT", "Issue PAT")
// where simpler English would do.
//
// Rules locked in here, smallest set that captures the intent:
//
//   1. The first prominent surface where the acronym appears (the
//      tab label) must expand it: "Access tokens (PAT)".
//   2. The page subtitle leads with the user-facing concept ("access
//      tokens"), not the bare acronym.
//   3. "mint(s) a PAT" disappears from any visible copy — it's a
//      backend implementation detail.
//   4. The headline issuance affordance ("Issue PAT") becomes a
//      readable verb phrase.
test.describe("enrollment page — PAT acronym expansion", () => {
    test("first prominent PAT mention is expanded and verbs are humanised", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/enrollment");

        // Wait for the tabs to render so the page-level copy is mounted.
        const accessTokensTab = page.getByRole("tab", { name: /access tokens/i });
        await expect(accessTokensTab).toBeVisible({ timeout: 10_000 });

        // The tab label must contain BOTH "Access tokens" (the user-facing
        // concept) and "PAT" (the acronym, in parens, so engineers and
        // docs can still grep for it).
        const tabLabel = (await accessTokensTab.textContent())?.trim() ?? "";
        expect(tabLabel).toMatch(/access tokens/i);
        expect(tabLabel).toMatch(/\(PAT\)/);

        // Subtitle (rendered on the default tab) must not say "raw PATs"
        // any more — "access tokens" is the user-facing concept.
        const subtitleArea = (await page.locator("body").textContent()) ?? "";
        expect(subtitleArea, "subtitle uses 'access tokens' wording").toMatch(
            /access tokens/i,
        );
        // "mints" is implementation jargon — must be gone from the
        // user surface, including the install-command dialog desc which
        // is reachable from the default tab.
        expect(subtitleArea, "no 'mint a PAT' jargon left on the page").not.toMatch(
            /mints? a PAT/i,
        );

        // Switch into the tokens tab. The headline issuance button on
        // that tab must use a real verb phrase, not bare "Issue PAT".
        await accessTokensTab.click();
        const issueButton = page.getByRole("button", {
            name: /issue (an? )?access token/i,
        });
        await expect(issueButton).toBeVisible({ timeout: 10_000 });
        await issueButton.click();

        // Dialog opens; its title leads with the noun phrase, not "PAT".
        const dialogTitle = page.getByRole("dialog").getByRole("heading").first();
        await expect(dialogTitle).toBeVisible();
        const titleText = (await dialogTitle.textContent())?.trim() ?? "";
        expect(titleText).toMatch(/access token/i);
        expect(titleText, "dialog title must not be bare 'Issue PAT'").not.toMatch(
            /^Issue PAT$/i,
        );
    });
});
