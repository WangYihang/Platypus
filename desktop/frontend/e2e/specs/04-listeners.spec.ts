import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("listeners", () => {
    test("list shows seeded listener + always-visible New listener button", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Listeners$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/listeners$/);

        // PageHeader has the always-visible primary action
        const newBtn = page.getByRole("button", { name: /New listener/ }).first();
        await expect(newBtn).toBeVisible();

        // Seeded listener row
        await expect(page.getByText("127.0.0.1:13399")).toBeVisible();
        await expect(page.getByText("listening").first()).toBeVisible();

        await page.screenshot({
            path: shotPath("05-listeners-list.png"),
            fullPage: false,
        });
    });

    test("create listener modal", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Listeners$/ }).click();

        await page.getByRole("button", { name: /New listener/ }).first().click();
        await expect(page.getByRole("dialog", { name: "New listener" })).toBeVisible();
        // The form has Host + Port fields with sensible defaults.
        await expect(page.getByLabel("Host")).toHaveValue("0.0.0.0");
        await page.screenshot({
            path: shotPath("07-listener-create-modal.png"),
            fullPage: false,
        });
        await page.getByRole("button", { name: "Cancel" }).click();
    });

    test("listener detail page", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Listeners$/ }).click();

        // Click the seeded row → detail page
        await page.getByText("127.0.0.1:13399").click();
        await expect(page).toHaveURL(/\/projects\/default\/listeners\/[^/]+$/);
        await expect(page.getByText("Listener", { exact: true })).toBeVisible();
        await expect(page.getByText("host:port")).toBeVisible();
        await expect(page.getByRole("button", { name: /Stop listener/ })).toBeVisible();
        await page.screenshot({
            path: shotPath("06-listener-detail.png"),
            fullPage: false,
        });
    });
});
