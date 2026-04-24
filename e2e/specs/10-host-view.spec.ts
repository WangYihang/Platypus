import { expect, test } from "@playwright/test";
import { loginAsAdmin, shotPath } from "../fixtures/auth";

test.describe("host view", () => {
    test("click row lands on Terminal tab with header + chips", async ({
        page,
    }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Hosts$/ }).click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts$/);

        // Click into the host row.
        const row = page.locator("table tbody tr").first();
        await expect(row).toBeVisible({ timeout: 10_000 });
        await row.click();
        await expect(page).toHaveURL(/\/projects\/default\/hosts\/[^/]+\/terminal$/);

        // PageHeader subtitle on HostView is "<N> active · <os>".
        await expect(
            page.getByText(/active · /).first(),
        ).toBeVisible({ timeout: 10_000 });

        // Current tab strip (Terminal/Files/Sessions/Info) — Tunnels
        // was removed with the v2 stack refactor.
        for (const label of ["Terminal", "Files", "Sessions", "Info"]) {
            await expect(page.getByRole("tab", { name: label })).toBeVisible();
        }

        await page.screenshot({
            path: shotPath("15-host-info.png"),
            fullPage: false,
        });
    });
});
