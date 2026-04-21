import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";
import { baseURL } from "../fixtures/env";

test.describe("auth", () => {
    test("login screen renders", async ({ page }) => {
        await page.goto(`${baseURL}/login`);
        await expect(
            page.getByRole("heading", { name: "Platypus" }),
        ).toBeVisible();
        await expect(page.getByText("Log in", { exact: true }).first()).toBeVisible();
        await expect(page.getByText("First-time setup")).toBeVisible();
        await page.screenshot({ path: shotPath("01-login.png"), fullPage: false });
    });

    test("login lands on /projects", async ({ page }) => {
        await loginAsAdmin(page);
        await expect(page).toHaveURL(/\/projects$/);
        // Sidebar brand visible.
        await expect(page.getByText("Platypus", { exact: true })).toBeVisible();
        await page.screenshot({
            path: shotPath("02-projects-landing.png"),
            fullPage: false,
        });
    });
});
