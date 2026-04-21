import { rmSync } from "node:fs";

// globalTeardown runs once after all specs. Kills the backend started
// by globalSetup and removes the temp dir (config + SQLite). Frontend
// is managed by Playwright's webServer and shuts down automatically.
export default async function globalTeardown() {
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
