import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The Fleet → Graph view's empty-state used to say "No agents
// currently connected to this project." even though the project
// could have agents enrolled and reachable — the topology snapshot
// was empty because mesh hadn't formed any links yet, not because
// agents weren't connected. The copy was actively misleading
// (operators saw it and assumed their agent was offline). Replace
// with wording that names the actual condition: no topology data
// yet, mesh links appear here as agents connect.
test.describe("graph empty-state wording", () => {
    test("does not claim no agents are connected", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();
        await page.getByRole("radio", { name: /Graph/ }).click();
        await expect(page).toHaveURL(/view=graph/);

        const panel = page.getByTestId("fleet-panel-graph");
        await expect(panel).toBeVisible();
        const text = (await panel.textContent()) ?? "";
        expect(text).not.toMatch(/no agents currently connected/i);
    });
});
