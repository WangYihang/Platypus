import { expect, test } from "@playwright/test";
import { startZeroConfigAgent } from "../fixtures/agent";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("mesh automatic discovery", () => {
    test("two zero-config agents bootstrap and discover each other via mDNS", async ({ page }) => {
        const projects = JSON.parse(
            process.env.PLATYPUS_E2E_PROJECTS || "[]",
        ) as Array<{ slug: string; id: string }>;
        const def = projects.find((p) => p.slug === "default");
        if (!def) throw new Error("default project missing");
        const token = process.env.PLATYPUS_E2E_ADMIN_TOKEN!;

        // 1. Start two agents with ZERO mesh configuration.
        // They should bootstrap from the server and then discover each other via mDNS.
        const agentA = await startZeroConfigAgent(def.id, token, {
            token: "agent-zero-a",
        });

        const agentB = await startZeroConfigAgent(def.id, token, {
            token: "agent-zero-b",
        });

        try {
            await loginAsAdmin(page);
            await page.getByRole("button", { name: /Default created/i }).click();
            await page.getByRole("link", { name: /Topology$/ }).click();
            await expect(page).toHaveURL(/\/projects\/default\/topology$/);

            // Give mDNS discovery and mesh links time to form and propagate.
            // mDNS might take a few seconds to scan and dial.
            await page.waitForTimeout(10000);

            // The topology page should show at least 3 machines (baseline + A + B).
            // And since they are meshed, they should be connected in the graph.
            // We'll check the machine count in the header.
            await expect(page.getByText(/\d+ machines/)).toBeVisible({ timeout: 30_000 });
            
            // Extract the number of machines.
            const headerText = await page.getByText(/\d+ machines/).innerText();
            const count = parseInt(headerText.split(" ")[0]);
            expect(count).toBeGreaterThanOrEqual(3);

            // If discovery worked, there should be links in the topology data.
            // We can check if "hub-and-spoke" message is GONE.
            await expect(page.getByText(/hub-and-spoke/)).not.toBeVisible();

            await page.screenshot({
                path: shotPath("23-mesh-discovery.png"),
                fullPage: false,
            });
        } finally {
            await agentB.kill();
            await agentA.kill();
        }
    });
});
