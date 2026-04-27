import { expect, test } from "../fixtures/test";

// The shadcn button variants render their keyboard focus ring through
// the --ring CSS variable. Pre-fix, --ring resolved to
// --color-border-strong (#525252) — a desaturated gray that sits on a
// dark gray button at <1.5:1 contrast (well under WCAG 2.4.7's
// "focus visible" floor of 3:1) and is essentially invisible on the
// primary near-white buttons.
//
// Lock --ring to a saturated colour bright enough to clear the AA
// floor on dark surfaces. We test the resolved variable rather than
// every individual button render: one CSS source, one assertion.
test.describe("focus ring contrast", () => {
    test("--ring resolves to a saturated colour, not the dim border-strong gray", async ({
        page,
    }) => {
        await page.goto("/");

        const ring = await page.evaluate(() => {
            const v = getComputedStyle(document.documentElement)
                .getPropertyValue("--ring")
                .trim();
            // Compute the actual rendered color in a sentinel element so
            // we can read it through Element.getPropertyValue with the
            // var() chain resolved.
            const sentinel = document.createElement("div");
            sentinel.style.color = `var(--ring)`;
            document.body.appendChild(sentinel);
            const resolved = getComputedStyle(sentinel).color;
            sentinel.remove();
            return { raw: v, resolved };
        });

        // The resolved color must NOT be the dim border-strong gray
        // (#525252 → rgb(82, 82, 82)). That value alone fails WCAG AA
        // on dark surfaces.
        expect(ring.resolved.replace(/\s+/g, "")).not.toBe("rgb(82,82,82)");

        // Belt: it should also not resolve to "transparent" / unset, and
        // should be a saturated colour. We accept any rgb() with at
        // least one channel that's clearly saturated relative to the
        // others (max - min >= 80) — captures both blue (#0070f3 →
        // rgb(0, 112, 243), max-min = 243) and any future bright red /
        // green / purple choice without pinning to a specific hex.
        const m = ring.resolved.match(/^rgba?\((\d+),\s*(\d+),\s*(\d+)/);
        expect(m, `--ring resolved to ${ring.resolved}; expected an rgb()`).not.toBeNull();
        const [r, g, b] = m!.slice(1, 4).map(Number);
        const spread = Math.max(r, g, b) - Math.min(r, g, b);
        expect(
            spread,
            `--ring is desaturated (channels=${[r, g, b].join(",")}); choose a saturated colour for focus visibility`,
        ).toBeGreaterThanOrEqual(80);
    });
});
