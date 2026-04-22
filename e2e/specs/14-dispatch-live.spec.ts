import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";
import { backendURL } from "../fixtures/env";
import { flagSessionForDispatch, listProjectSessions } from "../fixtures/api";

test.describe("dispatch with live session", () => {
    let liveSessionID = "";

    test.beforeAll(async () => {
        const projects = JSON.parse(
            process.env.PLATYPUS_E2E_PROJECTS || "[]",
        ) as Array<{ slug: string; id: string }>;
        const def = projects.find((p) => p.slug === "default");
        const token = process.env.PLATYPUS_E2E_ADMIN_TOKEN;
        if (!def || !token) {
            throw new Error("globalSetup didn't export project / admin token");
        }
        // Re-resolve the *current* live session — the agent may have
        // reconnected after globalSetup, in which case the original
        // BASELINE_SESSION is now disconnected and a fresh row exists.
        const live = await listProjectSessions(backendURL, token, def.id, {
            live: true,
        });
        if (live.length === 0) {
            throw new Error("no live sessions in default project — agent dead?");
        }
        liveSessionID = live[0].id;
        await flagSessionForDispatch(backendURL, token, liveSessionID, true);
    });

    test.afterAll(async () => {
        const token = process.env.PLATYPUS_E2E_ADMIN_TOKEN;
        if (liveSessionID && token) {
            try {
                await flagSessionForDispatch(backendURL, token, liveSessionID, false);
            } catch {
                /* best-effort cleanup */
            }
        }
    });

    test("Run dispatches `id` and shows uid= in the results", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Dispatch$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/dispatch$/);

        // The form pre-fills `id` / 3 — just hit Run.
        await page.getByRole("button", { name: "Run", exact: true }).click();

        // Results card appears with one row containing uid=
        await expect(page.getByText(/Results/i).first()).toBeVisible({
            timeout: 15_000,
        });
        await expect(page.getByText(/uid=/).first()).toBeVisible({
            timeout: 10_000,
        });

        await page.screenshot({
            path: shotPath("19-dispatch-live.png"),
            fullPage: false,
        });
    });
});
