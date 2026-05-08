import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// `PAT` was the user-facing label for what is actually a one-shot
// agent-bootstrap secret. New users routinely guessed wrong (Personal /
// Project / Provider Access Token? a GitHub-style account token?). The
// step-1 fix renames the user surface to "Enrollment tokens" so the
// scope ("this is how an agent JOINS the fleet") reads off the label.
//
// Backend tables, code paths, and structured logs still call them PATs
// — that's an internal detail. The frontend never surfaces "PAT" or
// "access token" any more.
//
// Rules locked in here, smallest set that captures the intent:
//
//   1. The tab label says "Enrollment tokens" (no "PAT", no "access
//      token").
//   2. The visible page surface uses "enrollment tokens" wording, not
//      "access tokens".
//   3. "mint(s) a PAT" stays gone — implementation jargon, doesn't
//      belong on the user surface.
//   4. The headline issuance affordance reads as "Issue enrollment
//      token" so the verb phrase matches the noun.
test.describe("enrollment page — enrollment-tokens vocabulary", () => {
    test("tab + buttons + dialog use 'enrollment token' wording", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/enrollment");

        // Wait for the tabs to render so the page-level copy is mounted.
        const tokensTab = page.getByRole("tab", { name: /enrollment tokens/i });
        await expect(tokensTab).toBeVisible({ timeout: 10_000 });

        // The tab label is "Enrollment tokens" — no "PAT" parens, no
        // "access tokens" wording.
        const tabLabel = (await tokensTab.textContent())?.trim() ?? "";
        expect(tabLabel.toLowerCase()).toContain("enrollment tokens");
        expect(tabLabel).not.toMatch(/\(PAT\)/);
        expect(tabLabel.toLowerCase()).not.toContain("access tokens");

        // Subtitle (rendered on the default tab) leads with "enrollment
        // tokens" — no "raw access tokens", no "mint a PAT" jargon.
        const bodyText = (await page.locator("body").textContent()) ?? "";
        expect(bodyText.toLowerCase()).toContain("enrollment tokens");
        expect(bodyText, "no 'mint a PAT' jargon left on the page").not.toMatch(
            /mints? a PAT/i,
        );

        // Switch into the tokens tab. The headline issuance button uses
        // the new noun phrase.
        await tokensTab.click();
        const issueButton = page.getByRole("button", {
            name: /issue (an? )?enrollment token/i,
        });
        await expect(issueButton).toBeVisible({ timeout: 10_000 });
        await issueButton.click();

        // Dialog opens; its title leads with the noun phrase.
        const dialogTitle = page.getByRole("dialog").getByRole("heading").first();
        await expect(dialogTitle).toBeVisible();
        const titleText = (await dialogTitle.textContent())?.trim() ?? "";
        expect(titleText.toLowerCase()).toMatch(/enrollment token/);
        expect(titleText, "dialog title must not say 'access token'").not.toMatch(
            /access token/i,
        );
    });
});
