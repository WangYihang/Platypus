import { expect, test } from "@playwright/test";
import { startMeshAgent } from "../fixtures/agent";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("mesh topology", () => {
    test("two agents form a mesh link and it appears in the UI", async ({ page }) => {
        const projects = JSON.parse(
            process.env.PLATYPUS_E2E_PROJECTS || "[]",
        ) as Array<{ slug: string; id: string }>;
        const def = projects.find((p) => p.slug === "default");
        if (!def) throw new Error("default project missing");
        const token = process.env.PLATYPUS_E2E_ADMIN_TOKEN!;

        // 1. Start Agent A (Mesh Listener)
        const agentA = await startMeshAgent(def.id, token, {
            meshListen: "127.0.0.1:17771",
        });

        // 2. Start Agent B (Mesh Dialer, peers to A)
        const agentB = await startMeshAgent(def.id, token, {
            meshListen: "127.0.0.1:17772",
            peers: ["127.0.0.1:17771"],
        });

        try {
            await loginAsAdmin(page);
            await page.getByRole("button", { name: /Default created/i }).click();
            await page.getByRole("link", { name: /Topology$/ }).click();
            await expect(page).toHaveURL(/\/projects\/default\/topology$/);

            // Give mesh links and machines time to register and propagate to UI via WS.
            await page.waitForTimeout(5000);

            // The topology page shows stats in the header.
            // Wait for at least 3 machines (baseline + A + B).
            await expect(page.getByText(/\d+ machines/)).toBeVisible({ timeout: 25_000 });
            
            // In mesh mode, the header doesn't show "hub-and-spoke".
            await expect(page.getByText(/hub-and-spoke/)).not.toBeVisible();

            await page.screenshot({
                path: shotPath("22-mesh-topology.png"),
                fullPage: false,
            });
        } finally {
            await agentB.kill();
            await agentA.kill();
        }
    });
});
