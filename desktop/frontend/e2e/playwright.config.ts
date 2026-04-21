import { defineConfig, devices } from "@playwright/test";

import {
    FRONTEND_DIR,
    FRONTEND_HOST,
    FRONTEND_PORT,
    baseURL,
    backendURL,
} from "./fixtures/env";

// E2E config. globalSetup spawns the backend itself (because we need
// to capture the bootstrap secret from its stdout); Playwright's
// webServer only manages the frontend.
//
// Two test runs:
//   - default — assertions only, fail-on-first-error semantics
//   - --project=screenshots — same specs but writes PNGs into
//     docs/screenshots/ for the gallery
export default defineConfig({
    testDir: "./specs",
    fullyParallel: false, // shared backend DB; serial runs are fine and avoid races
    workers: 1,
    timeout: 60_000,
    expect: { timeout: 10_000 },
    reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
    use: {
        baseURL,
        trace: "retain-on-failure",
        screenshot: "only-on-failure",
        video: "retain-on-failure",
        // Force a stable viewport so screenshots come out the same size
        // every run. 1440x900 is a comfortable laptop screen.
        viewport: { width: 1440, height: 900 },
        actionTimeout: 10_000,
        navigationTimeout: 15_000,
        // Used by specs via `process.env.E2E_BACKEND_URL`.
        extraHTTPHeaders: {},
    },
    projects: [
        {
            name: "chromium",
            use: { ...devices["Desktop Chrome"] },
        },
    ],
    globalSetup: "./global-setup.ts",
    globalTeardown: "./global-teardown.ts",
    webServer: {
        // Frontend only. Backend is launched by globalSetup so we can
        // capture stdout for the bootstrap secret.
        command: `npm run dev -- --host ${FRONTEND_HOST} --port ${FRONTEND_PORT} --strictPort`,
        cwd: FRONTEND_DIR,
        url: baseURL,
        timeout: 60_000,
        reuseExistingServer: !process.env.CI,
        stdout: "ignore",
        stderr: "pipe",
        env: { E2E_BACKEND_URL: backendURL },
    },
});
