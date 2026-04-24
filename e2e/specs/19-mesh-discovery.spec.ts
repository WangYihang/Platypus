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
        const agentA = await startZeroConfigAgent(def.id, token);
        const agentB = await startZeroConfigAgent(def.id, token);

        try {
            await loginAsAdmin(page);
            await page.getByRole("button", { name: /Default created/i }).click();
            await page.getByRole("link", { name: /Topology$/ }).click();
            await expect(page).toHaveURL(/\/projects\/default\/topology$/);

            // Wait for the topology page to show at least 3 machines (baseline + A + B).
            // We use a longer timeout and polling because mDNS and Topology streams are asynchronous.
            const machineCountLabel = page.getByText(/\d+ machines/);
            await expect(machineCountLabel).toBeVisible({ timeout: 45_000 });
            
            // Periodically check the count until it's at least 3.
            await expect(async () => {
                const headerText = await machineCountLabel.innerText();
                const count = parseInt(headerText.split(" ")[0]);
                expect(count).toBeGreaterThanOrEqual(3);
            }).toPass({ timeout: 30_000 });

            // If discovery worked, there should be links in the topology data.
            // In mesh mode, the "hub-and-spoke" warning should be hidden.
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
