import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// The graph (mesh topology) view's empty-state used to say "No
// agents currently connected to this project." even though the
// project could have agents enrolled and reachable — the topology
// snapshot was empty because mesh hadn't formed any links yet, not
// because agents weren't connected. The copy was actively
// misleading (operators saw it and assumed their agent was
// offline). Replace with wording that names the actual condition:
// no topology data yet, mesh links appear here as agents connect.
//
// After the Fleet → Hosts IA split the graph moved to its own
// /hosts/topology sub-route (was a `?view=graph` toggle on Fleet).
test.describe("graph empty-state wording", () => {
    test("does not claim no agents are connected", async ({ page }) => {
        await loginAsAdmin(page);
        await page.goto("/projects/default/hosts/topology");
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/topology$/);

        const text = (await page.locator("main").textContent()) ?? "";
        expect(text).not.toMatch(/no agents currently connected/i);
    });
});
