import { describe, expect, it } from "vitest";
import { humanizeError } from "./humanizeError";

describe("humanizeError — plugin_not_installed", () => {
    // The agent emits "plugin_not_installed: <plugin_id>" whenever a
    // stream type lands at the dispatcher with no claim — typically
    // because the operator's enroll-time baseline allowlist didn't
    // include the wasm plugin that owns the capability. Map that to
    // a one-line action prompt so users don't have to grep for the
    // server log.
    it("maps the bare error to an actionable hint", () => {
        const out = humanizeError(
            new Error("plugin_not_installed: com.platypus.sys-file-read"),
        );
        expect(out).toBe(
            'This action needs the "sys-file-read" plugin. Install it from the Plugins tab.',
        );
    });

    // The marker survives wrapping the agent error in the REST layer
    // ("502: ...") so the parser must look anywhere inside the message.
    it("matches inside an HTTP-shaped error", () => {
        const out = humanizeError(
            new Error(
                '502: {"error":"plugin_not_installed: com.platypus.sys-process-open"}',
            ),
        );
        expect(out).toBe(
            'This action needs the "sys-process-open" plugin. Install it from the Plugins tab.',
        );
    });

    // Non-com.platypus.* publisher should round-trip the full id so
    // the user can find the plugin in the marketplace under the
    // exact name. Today we don't have third-party stream-claiming
    // plugins, but the format is stable.
    it("preserves the full id when the prefix isn't com.platypus.", () => {
        const out = humanizeError(
            new Error("plugin_not_installed: org.example.cool-tool"),
        );
        expect(out).toBe(
            'This action needs the "org.example.cool-tool" plugin. Install it from the Plugins tab.',
        );
    });

    // The mapping is precedence-checked before the generic HTTP path —
    // a plain 502 with this body must NOT fall through to the
    // "Server error (502) — try again in a moment." branch, which
    // would tell the operator to retry an action that can never
    // succeed without first installing a plugin.
    it("wins over the 5xx generic message", () => {
        const out = humanizeError(
            new Error(
                '502: {"error":"plugin_not_installed: com.platypus.sys-tunnel-pull"}',
            ),
        );
        expect(out).not.toMatch(/Server error/i);
    });

    // Sanity: an unrelated error still falls through to the
    // existing handlers. Otherwise a future regression in the parser
    // could swallow real network errors.
    it("ignores unrelated errors", () => {
        expect(humanizeError(new TypeError("Failed to fetch"))).toBe(
            "Cannot reach the server. Check your connection.",
        );
    });
});

// The fallback path used to end in String(e), so anything arriving as
// a plain object — a parsed JSON error body, a rejected non-Error —
// reached the user's toast as the literal "[object Object]". That is
// worse than the raw message this module was written to replace.
describe("humanizeError on non-Error throws", () => {
    it("never renders [object Object]", () => {
        for (const thrown of [
            { error: "insufficient role" },
            { message: "boom" },
            { detail: "nope" },
            { status: 500 },
            {},
            [1, 2, 3],
        ]) {
            expect(humanizeError(thrown)).not.toMatch(/\[object Object\]/);
        }
    });

    it("prefers a message-shaped field over raw JSON", () => {
        expect(humanizeError({ error: "insufficient role" })).toMatch(/insufficient role/i);
        expect(humanizeError({ message: "disk full" })).toMatch(/disk full/i);
    });

    it("falls back to something actionable when the object carries nothing", () => {
        const out = humanizeError({});
        expect(out.trim()).not.toBe("");
        expect(out).not.toMatch(/\[object Object\]/);
    });

    it("handles null, undefined and circular objects without throwing", () => {
        const circular: Record<string, unknown> = {};
        circular.self = circular;
        for (const thrown of [null, undefined, circular]) {
            expect(() => humanizeError(thrown)).not.toThrow();
            expect(humanizeError(thrown).trim()).not.toBe("");
        }
    });

    // plugin_not_installed detection reads the same text. An object-
    // shaped error used to stringify to "[object Object]" before the
    // regex ran, so the marker could never match and the operator got
    // a generic message instead of "install this plugin".
    it("still detects plugin_not_installed inside an object-shaped error", () => {
        const out = humanizeError({ error: "plugin_not_installed: sys-file-read" });
        expect(out).toMatch(/sys-file-read/i);
    });
});
