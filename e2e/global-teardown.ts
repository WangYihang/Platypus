import { rmSync } from "node:fs";

// globalTeardown runs once after all specs. Kills the baseline agent
// (cleanly, so the disconnect path runs), then the backend, then
// removes the temp dir. Frontend is managed by Playwright's webServer.
export default async function globalTeardown() {
    const agentPidStr = process.env.PLATYPUS_E2E_AGENT_PID;
    if (agentPidStr) {
        const pid = Number(agentPidStr);
        try {
            // Agent handles SIGTERM cleanly (cmd/platypus-agent/main.go:30).
            // Give it 2s to drain via MarkSessionDisconnected; escalate
            // if it hangs.
            process.kill(pid, "SIGTERM");
            await waitForExit(pid, 2_000);
        } catch {
            /* already gone */
        }
        try {
            process.kill(pid, "SIGKILL");
        } catch {
            /* already gone */
        }
    }

    const pidStr = process.env.PLATYPUS_E2E_PID;
    if (pidStr) {
        const pid = Number(pidStr);
        try {
            // platypus-server prompts "Exit? [Y/N]" on SIGTERM/SIGINT
            // (interactive shell ergonomics). Stdin is closed in tests
            // so the prompt loops forever; skip the polite signal and
            // SIGKILL directly. The temp DB gets removed below anyway.
            process.kill(pid, "SIGKILL");
        } catch (e) {
            // PID gone is fine — most likely it crashed and we missed it.
            if (process.env.E2E_VERBOSE_BACKEND) {
                console.warn(`[e2e] could not signal backend pid=${pid}: ${String(e)}`);
            }
        }
    }

    const tmpdir = process.env.PLATYPUS_E2E_TMPDIR;
    if (tmpdir) {
        try {
            rmSync(tmpdir, { recursive: true, force: true });
        } catch {
            // best-effort cleanup; don't fail the run on a stuck file.
        }
    }
}

// waitForExit polls until `process.kill(pid, 0)` throws ESRCH, signalling
// the process has gone. Used to give the agent a moment after SIGTERM
// before escalating to SIGKILL.
async function waitForExit(pid: number, timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        try {
            process.kill(pid, 0);
        } catch {
            return;
        }
        await new Promise((r) => setTimeout(r, 50));
    }
    throw new Error(`pid ${pid} did not exit within ${timeoutMs}ms`);
}
