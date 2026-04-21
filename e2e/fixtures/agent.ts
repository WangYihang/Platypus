import { ChildProcess, spawn } from "node:child_process";
import { test as base } from "@playwright/test";

import { AGENT_BINARY, BACKEND_HOST, SEEDED_LISTENER_PORT, backendURL } from "./env";
import { listProjectSessions, waitForSessions } from "./api";

// Per-spec helpers for spawning extra platypus-agent processes beyond
// the baseline one started in globalSetup. Specs that need to assert
// multi-agent behaviour pull in the `liveAgent` fixture; others can
// call startExtraAgent directly inside beforeAll/afterAll.

export interface AgentHandle {
    pid: number;
    proc: ChildProcess;
    kill: () => Promise<void>;
}

// startExtraAgent spawns a platypus-agent connecting to the seeded
// listener and waits until total live sessions in the default project
// have grown by 1. Caller is responsible for kill().
export async function startExtraAgent(
    projectID: string,
    adminToken: string,
): Promise<AgentHandle> {
    const before = await listProjectSessions(backendURL, adminToken, projectID, {
        live: true,
    });
    const proc = spawn(
        AGENT_BINARY,
        [
            "--host",
            BACKEND_HOST,
            "--port",
            String(SEEDED_LISTENER_PORT),
            "--token",
            "e2e-extra",
        ],
        { stdio: ["ignore", "pipe", "pipe"], env: { ...process.env } },
    );
    if (!proc.pid) {
        throw new Error("failed to spawn extra agent (no pid)");
    }
    if (process.env.E2E_VERBOSE_AGENT) {
        proc.stdout?.on("data", (c: Buffer) => process.stdout.write(`[agent+] ${c}`));
        proc.stderr?.on("data", (c: Buffer) => process.stderr.write(`[agent+!] ${c}`));
    }
    await waitForSessions(
        backendURL,
        adminToken,
        projectID,
        before.length + 1,
        15_000,
    );
    return {
        pid: proc.pid,
        proc,
        async kill() {
            try {
                process.kill(proc.pid!, "SIGTERM");
            } catch {
                /* gone */
            }
            await new Promise<void>((resolve) => {
                if (proc.exitCode !== null || proc.signalCode) return resolve();
                const t = setTimeout(() => {
                    try {
                        process.kill(proc.pid!, "SIGKILL");
                    } catch {
                        /* gone */
                    }
                    resolve();
                }, 2_000);
                proc.once("exit", () => {
                    clearTimeout(t);
                    resolve();
                });
            });
        },
    };
}

// liveAgent fixture: spawns one extra agent per spec that imports it.
// Reads project ID + admin token from globalSetup-exported env.
export const test = base.extend<{ liveAgent: AgentHandle }>({
    liveAgent: async ({}, use) => {
        const projects = JSON.parse(
            process.env.PLATYPUS_E2E_PROJECTS || "[]",
        ) as Array<{ slug: string; id: string }>;
        const def = projects.find((p) => p.slug === "default");
        if (!def) throw new Error("default project missing from PLATYPUS_E2E_PROJECTS");
        const token = process.env.PLATYPUS_E2E_ADMIN_TOKEN;
        if (!token) throw new Error("PLATYPUS_E2E_ADMIN_TOKEN not set");
        const handle = await startExtraAgent(def.id, token);
        await use(handle);
        await handle.kill();
    },
});

export { expect } from "@playwright/test";
