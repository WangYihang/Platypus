// leafText renders an unknown JSON-ish value as text for a form field
// or a read-only display.
//
// The obvious String(v) is wrong for anything that isn't a primitive:
// it produces the literal "[object Object]", which tells the reader
// nothing and — in an editable field — is what gets saved back if they
// don't notice. That matters here because several of these values are
// typed `unknown` on purpose (settings descriptors, plugin config
// schemas), so a non-primitive is a shape the UI is expected to
// survive, not a can't-happen.
//
// Objects and arrays render as JSON instead. An operator looking at a
// malformed plugin config or an unexpected setting value can at least
// see what it actually is.
export function leafText(v: unknown): string {
    if (v === null || v === undefined) return "";
    if (typeof v === "string") return v;
    if (typeof v === "number" || typeof v === "boolean" || typeof v === "bigint") {
        return String(v);
    }
    try {
        return JSON.stringify(v) ?? "";
    } catch {
        // Circular, or a BigInt nested somewhere JSON.stringify chokes
        // on. Nothing readable to offer, so say nothing.
        return "";
    }
}
