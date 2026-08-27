import base from "./playwright.config";
import type { PlaywrightTestConfig } from "@playwright/test";
const EXE = "/opt/pw-browsers/chromium-1194/chrome-linux/chrome";
const config: PlaywrightTestConfig = {
    ...base,
    projects: (base.projects ?? []).map((p) => ({
        ...p,
        use: { ...p.use, launchOptions: { ...(p.use as any)?.launchOptions, executablePath: EXE } },
    })),
};
export default config;
