import { expect, test } from "../fixtures/test";

// F7: --color-main and --color-surface used to be #111 and #1a1a1a, a
// 1-hex-unit difference that's invisible on most monitors. The whole
// "card floats above the page background" affordance was lost. We
// don't pick the exact hex values in the test (so a future tweak
// stays free), but we DO require the two surfaces to be measurably
// different — at least 8 units apart on each RGB channel — so the
// regression can't sneak back in via another "small tweak".
test.describe("surface vs main contrast", () => {
    test("card surface is visually distinguishable from page background", async ({
        page,
    }) => {
        await page.goto("/");

        const colors = await page.evaluate(() => {
            function resolve(varName: string): string {
                const sentinel = document.createElement("div");
                sentinel.style.color = `var(${varName})`;
                document.body.appendChild(sentinel);
                const c = getComputedStyle(sentinel).color;
                sentinel.remove();
                return c;
            }
            return {
                main: resolve("--color-main"),
                surface: resolve("--color-surface"),
            };
        });

        function rgb(s: string): [number, number, number] {
            const m = s.match(/(\d+),\s*(\d+),\s*(\d+)/);
            if (!m) throw new Error(`unparseable color ${s}`);
            return [Number(m[1]), Number(m[2]), Number(m[3])];
        }

        const [mr, mg, mb] = rgb(colors.main);
        const [sr, sg, sb] = rgb(colors.surface);
        const channelDelta = Math.max(
            Math.abs(sr - mr),
            Math.abs(sg - mg),
            Math.abs(sb - mb),
        );

        // 12 units on a 0-255 channel ~= 5% lightness. The previous
        // values (#111 vs #1a1a1a) differed by 9 per channel, which on
        // average displays / typical room lighting reads as one flat
        // surface — the "card floats above the page" affordance was
        // gone. Bumping the floor to 12 keeps room for future tweaks
        // without making this regression possible again.
        expect(
            channelDelta,
            `--color-main=${colors.main} vs --color-surface=${colors.surface} only differ by ${channelDelta} per channel; cards won't read as floating`,
        ).toBeGreaterThanOrEqual(12);
    });
});
