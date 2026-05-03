import { test } from "@playwright/test";

import { loginAsAdmin } from "../../fixtures/auth";
import { caption, pause } from "../../fixtures/demo";

// 07-marketplace — narrated walk through the new global Marketplace
// tab shipped with the plugin runtime work. Today the catalog is
// empty in dev (PLATYPUS_PLUGIN_INDEX unconfigured) so the demo
// focuses on the page chrome + the refresh + search interactions;
// once a real index URL is wired the demo will extend with "browse
// → click into a plugin → install on a host" once the per-host
// install dialog ships.
test("walk: open the marketplace tab and refresh the catalog", async ({ page }) => {
    await loginAsAdmin(page);
    await pause(page, 800);

    await caption(
        page,
        "Marketplace is a global tab — pick plugins to install on any host.",
        1500,
    );
    await page.getByRole("link", { name: /^Marketplace$/ }).click();
    await pause(page, 1000);

    await caption(
        page,
        "The catalog is a SQLite mirror of the platypus-plugins git index.",
        1600,
    );
    await pause(page, 600);

    await caption(
        page,
        "Refresh hits the index URL and rebuilds the cache (admin only).",
        1500,
    );
    await page.getByRole("button", { name: /^Refresh$/ }).click();
    await pause(page, 1500);

    await caption(
        page,
        "Search filters the latest version of every plugin by name.",
        1400,
    );
    const search = page.getByPlaceholder(/Search by plugin name/i);
    await search.click();
    await search.pressSequentially("nginx", { delay: 80 });
    await pause(page, 1200);

    await caption(
        page,
        "Each card shows declared capabilities — the operator confirms grants on install.",
        1800,
    );
    await pause(page, 1500);
});
