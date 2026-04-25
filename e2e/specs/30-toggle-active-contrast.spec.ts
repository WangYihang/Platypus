import { expect, test } from "../fixtures/test";

import { loginAsAdmin } from "../fixtures/auth";

// Regression: shadcn ToggleGroup's data-[state=on] state mapped both
// `bg-accent` and `text-accent-foreground` to the same near-white
// token via @theme inline collision. Result: white text on white
// background — the active toggle vanished. Assert background and
// foreground colours differ wherever a ToggleGroup is rendered active.
test.describe("toggle-group active state contrast", () => {
    test("Fleet view-switcher 'Table' toggle has visible text", async ({ page }) => {
        await loginAsAdmin(page);
        await page.getByRole("button", { name: /Default created/i }).click();
        await page.getByRole("link", { name: /Fleet$/ }).click();

        const active = page.getByRole("radio", { name: /Table/ });
        await expect(active).toHaveAttribute("data-state", "on");

        const colors = await active.evaluate((el) => {
            const cs = window.getComputedStyle(el);
            return { bg: cs.backgroundColor, fg: cs.color };
        });
        expect(colors.bg).not.toBe(colors.fg);
    });
});
