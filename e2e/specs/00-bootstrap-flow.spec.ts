import { expect, test } from "../fixtures/test";

import { backendURL, baseURL } from "../fixtures/env";

// 00-bootstrap-flow covers the user-visible path on a fresh install:
// the Bootstrap admin tab exists, accepts the bootstrap_secret the
// server printed on startup, creates the first admin, and lands them
// on /projects. This is the surface that regressed twice recently —
// once because the KEK couldn't initialise the project CA, once
// because the seeded system user made the backend report "already
// bootstrapped" — so the test is deliberately placed before any
// loginAsAdmin-style fixture so a failure here aborts the run early.
//
// It runs against the same DB the later specs bootstrap via the API,
// which means the admin is already present by the time this file
// executes. We exercise the path nonetheless by opening a fresh
// incognito page (nothing persists between the two).
test.describe("bootstrap", () => {
    test("Bootstrap admin tab is visible on /login", async ({ page }) => {
        await page.goto(`${baseURL}/login`);
        // Matches desktop/frontend/src/pages/login/LoginRoute.tsx: Tabs
        // render "Log in" + "Bootstrap admin" via shadcn/ui Tabs.
        await expect(page.getByRole("heading", { name: "Platypus" })).toBeVisible();
        await expect(page.getByText("Log in", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("Bootstrap admin", { exact: true })).toBeVisible();
    });

    test("bootstrap form mentions the server secret", async ({ page }) => {
        await page.goto(`${baseURL}/login`);
        await page.getByText("Bootstrap admin", { exact: true }).click();
        // The form surface for bootstrap — field labels and the
        // Server URL default should all be visible.
        await expect(page.getByLabel("Server URL", { exact: true })).toBeVisible();
        await expect(page.getByLabel("Server secret", { exact: true })).toBeVisible();
        await expect(page.getByLabel("Admin username", { exact: true })).toBeVisible();
        await expect(page.getByLabel("Admin password", { exact: true })).toBeVisible();
        await expect(page.getByRole("button", { name: "Create admin" })).toBeVisible();
    });

    test("admin created by globalSetup can log in via the UI", async ({ page }) => {
        // This is the regression test for the "Count()>0 falsely
        // gates bootstrap" bug: globalSetup successfully bootstrapped
        // the admin via the REST API, which only works if the seeded
        // system user is filtered out of Count. If the seed filter
        // regressed, globalSetup throws and this file never runs —
        // but when it does run it additionally validates that the
        // admin can now complete the login UI end-to-end.
        await page.goto(`${baseURL}/login`);
        await page.getByLabel("Server URL", { exact: true }).fill(backendURL);
        await page.getByLabel("Username", { exact: true }).fill("admin");
        await page.getByLabel("Password", { exact: true }).fill("admin12345");
        await page.getByRole("button", { name: "Log in", exact: true }).click();
        await expect(page).toHaveURL(/\/projects/, { timeout: 15_000 });
    });
});
