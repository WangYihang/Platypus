import { expect, test } from "../fixtures/test";

// F9: the colour tokens used to be:
//
//   palette.success    = #0070f3   ← actually blue
//   palette.successDot = #3ECF8E   ← actually green
//
// which made every consumer of "the success colour" surface a blue
// pill where users expected green, and the actual green was hidden
// behind the awkward `-Dot` suffix. The rename:
//
//   palette.success → green
//   palette.info    → blue
//
// Pin the resolved values via the CSS variable chain so a future
// rename can't silently swap them back. We don't pin the exact hex —
// design might want to tweak the green or blue — only the high-level
// "success is greener than blue, info is bluer than green" property.
test.describe("colour-token rename: success is green, info is blue", () => {
    test("success resolves to a more-green-than-blue colour", async ({ page }) => {
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
                success: resolve("--color-success"),
                info: resolve("--color-info"),
            };
        });

        function rgb(s: string): [number, number, number] {
            const m = s.match(/(\d+),\s*(\d+),\s*(\d+)/);
            if (!m) throw new Error(`unparseable colour ${s}`);
            return [Number(m[1]), Number(m[2]), Number(m[3])];
        }

        const [sR, sG, sB] = rgb(colors.success);
        const [iR, iG, iB] = rgb(colors.info);

        // Success: green channel must dominate.
        expect(
            sG,
            `success=${colors.success} but green isn't dominant`,
        ).toBeGreaterThan(sR);
        expect(
            sG,
            `success=${colors.success} but green isn't dominant`,
        ).toBeGreaterThan(sB);

        // Info: blue channel must dominate.
        expect(iB, `info=${colors.info} but blue isn't dominant`).toBeGreaterThan(iR);
        expect(iB, `info=${colors.info} but blue isn't dominant`).toBeGreaterThan(iG);
    });
});
