import { expect, test } from "@playwright/test";

import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("hosts", () => {
    test("empty state CTA points to listeners", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts$/);

        // Empty state — no agents have connected yet
        await expect(page.getByRole("heading", { level: 1, name: "Hosts" })
            .or(page.getByText("Hosts").first())).toBeVisible();
        await expect(page.getByText("No hosts yet")).toBeVisible();
        await expect(page.getByRole("button", { name: /Manage listeners/ })).toBeVisible();

        await page.screenshot({
            path: shotPath("08-hosts-empty.png"),
            fullPage: false,
        });
    });
});
