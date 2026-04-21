import { Page, expect } from "@playwright/test";

import {
    ADMIN_PASSWORD,
    ADMIN_USERNAME,
    backendURL,
    baseURL,
} from "./env";

// loginAsAdmin drives the login UI (NOT the API) so the post-login
// state is authentic for screenshots. Lands on /projects on success.
export async function loginAsAdmin(page: Page): Promise<void> {
    await page.goto(`${baseURL}/login`);
    // Make sure we're on the Log-in tab (default), then fill via labels —
    // antd Input.Password's nested wrapper makes positional input lookups
    // brittle, but the antd Form labels are stable.
    await page.getByLabel("Server URL", { exact: true }).fill(backendURL);
    await page.getByLabel("Username", { exact: true }).fill(ADMIN_USERNAME);
    await page.getByLabel("Password", { exact: true }).fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    await expect(page).toHaveURL(/\/projects/, { timeout: 15_000 });
}

// shotPath returns the absolute path inside docs/screenshots/ for the
// given filename. Specs use this so screenshot paths line up with the
// gallery generator's filename-prefix sort order.
import { SCREENSHOT_DIR } from "./env";
import * as path from "node:path";
export function shotPath(name: string): string {
    return path.join(SCREENSHOT_DIR, name);
}
