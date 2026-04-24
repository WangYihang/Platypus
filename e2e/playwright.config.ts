import { defineConfig, devices } from "@playwright/test";

import {
    FRONTEND_DIR,
    FRONTEND_HOST,
    FRONTEND_PORT,
    baseURL,
    backendURL,
} from "./fixtures/env";

// E2E config. globalSetup spawns the backend binary (so we can read
// its stdout for the seed secret) and seeds the DB.
export default defineConfig({
    testDir: "./specs",
    fullyParallel: false,
    forbidOnly: !!process.env.CI,
    retries: process.env.CI ? 2 : 0,
    workers: 1,
    reporter: [["html", { open: "never" }], ["list"]],
    timeout: 60_000,
    use: {
        baseURL,
        trace: "on",
        video: "on",
        // Backend ingress uses a self-signed cert when no real cert is
        // wired up (the docker/dev default). Without this flag the
        // browser blocks every HTTPS fetch the SPA makes to the API.
        ignoreHTTPSErrors: true,
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
        command: `cd ${FRONTEND_DIR} && pnpm run dev:web -- --host 127.0.0.1 --port 5173 --strictPort`,
        cwd: FRONTEND_DIR,
        url: baseURL,
        timeout: 90_000,
        reuseExistingServer: false,
        stdout: "ignore",
        stderr: "pipe",
        env: { E2E_BACKEND_URL: backendURL },
    },
});
