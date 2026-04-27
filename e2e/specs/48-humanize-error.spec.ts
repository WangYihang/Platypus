import { expect, test } from "../fixtures/test";

// F5: humanizeError replaces the previous toast.error(`x: ${String(e)}`)
// pattern with a small mapper that turns transport-shape errors into
// short, user-readable strings. The function is pure (no DOM / toast)
// so we exercise it via Vite's dev-time dynamic import in the page
// context — that lets us assert the mapping behaviour without
// duplicating it in TypeScript fixture code.
//
// Each row asserts: given an error fixture, humanizeError returns the
// expected user-facing string. The fixtures cover the 401 / 403 / 404 /
// 409 / 429 / 5xx / network / unknown / SessionExpiredError /
// StaleServerResponseError paths called out by the F5 plan item.

test.describe("humanizeError mapping", () => {
    test("maps each transport shape to its user-facing string", async ({ page }) => {
        // Open any SPA page so Vite can resolve /src/lib/humanizeError.ts
        // via the dev server.
        await page.goto("/");

        const out = await page.evaluate(async () => {
            const m = await import("/src/lib/humanizeError.ts");
            const cases: Array<{ name: string; err: unknown }> = [
                { name: "401", err: new Error('401: {"error":"unauthorized"}') },
                { name: "403", err: new Error('403: {"error":"insufficient role"}') },
                { name: "404", err: new Error("404: not found") },
                {
                    name: "409",
                    err: new Error('409: {"error":"slug already taken"}'),
                },
                {
                    name: "409-no-body",
                    err: new Error("409: "),
                },
                { name: "429", err: new Error("429: rate limited") },
                { name: "503", err: new Error("503: upstream timeout") },
                {
                    name: "418",
                    err: new Error('418: {"error":"i am a teapot"}'),
                },
                {
                    name: "418-no-body",
                    err: new Error("418: "),
                },
                {
                    name: "network-fetch",
                    err: new TypeError("Failed to fetch"),
                },
                {
                    name: "session-expired",
                    err: Object.assign(new Error("revoked"), {
                        name: "SessionExpiredError",
                    }),
                },
                {
                    name: "stale-server",
                    err: Object.assign(new Error("server switched"), {
                        name: "StaleServerResponseError",
                    }),
                },
                {
                    name: "raw",
                    err: new Error("Error: something broke"),
                },
                { name: "non-error", err: "plain string" },
            ];
            return cases.map((c) => ({
                name: c.name,
                got: m.humanizeError(c.err),
            }));
        });

        const want: Record<string, RegExp> = {
            "401": /session expired/i,
            "403": /don'?t have permission/i,
            "404": /no longer exists/i,
            "409": /conflict.*slug already taken/i,
            "409-no-body": /conflict/i,
            "429": /too many requests/i,
            "503": /server error \(503\)/i,
            "418": /i am a teapot/i,
            "418-no-body": /request failed \(418\)/i,
            "network-fetch": /cannot reach the server/i,
            "session-expired": /session expired/i,
            "stale-server": /switched servers/i,
            raw: /^Something broke$/i,
            "non-error": /^plain string$/i,
        };

        for (const row of out) {
            const re = want[row.name];
            expect(re, `no expectation registered for ${row.name}`).not.toBeUndefined();
            expect(row.got, `humanizeError(${row.name}) = ${row.got}`).toMatch(re);
            // Belt: result must never include the literal `Error:` prefix
            // or a JSON brace, which would mean we leaked the raw shape.
            expect(row.got, `${row.name} leaks raw error prefix`).not.toMatch(
                /^Error:/,
            );
            expect(
                row.got,
                `${row.name} leaks JSON shape`,
            ).not.toMatch(/^[{[]/);
        }
    });
});
