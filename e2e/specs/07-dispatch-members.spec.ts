import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("dispatch + members", () => {
    test("dispatch page renders the form", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Dispatch$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/dispatch$/);

        await expect(page.getByRole("heading", { name: "Dispatch", level: 1 })
            .or(page.getByText("Dispatch").first())).toBeVisible();
        await expect(page.getByText("Run command")).toBeVisible();
        await expect(page.getByLabel("Command")).toBeVisible();
        await expect(page.getByRole("button", { name: "Run" })).toBeVisible();

        await page.screenshot({
            path: shotPath("10-dispatch.png"),
            fullPage: false,
        });
    });

    test("members page lists the admin", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Members$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/members$/);

        // Admin is auto-added as a member of any project they create.
        await expect(page.getByText("admin").first()).toBeVisible();
        await expect(page.getByRole("button", { name: /Add member/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("11-members.png"),
            fullPage: false,
        });
    });
});
