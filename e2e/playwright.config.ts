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
            // Demos live under specs/_demo/ and are produced via
            // `pnpm run demos` (the dedicated `demo` project). Skip
            // them in the default regression run so a regular test
            // pass doesn't churn out 5 narrated videos.
            testIgnore: ["**/_demo/**"],
            use: { ...devices["Desktop Chrome"] },
        },
        {
            name: "demo",
            testDir: "./specs/_demo",
            // Demo specs use *.demo.ts (not *.spec.ts) so the default
            // glob would skip them. Match them explicitly here.
            testMatch: "**/*.demo.ts",
            use: {
                ...devices["Desktop Chrome"],
                viewport: { width: 1440, height: 900 },
                // 250ms slowMo turns "click → wait → type" into
                // something a viewer can follow. Each demo also
                // stages explicit pause()s and captions on top.
                launchOptions: { slowMo: 250 },
                video: { mode: "on", size: { width: 1440, height: 900 } },
                trace: "off",
            },
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
