import { describe, expect, it } from "vitest";

import { quickPathsForHost, QuickPath } from "./quickPaths";

// quickPathsForHost is the brain behind the FileBrowser chip row —
// pure data, derived from the agent-reported Host so we can render
// platform-appropriate shortcuts. Tested in isolation so future
// "Windows hosts get C:\ instead of /" lands as a contained patch.
//
// Contract pinned here:
//   1. Linux / macOS hosts get the canonical Unix shortcuts.
//   2. The home (~) chip resolves from `current_user`; when there's
//      no user the chip is omitted (we don't guess /home/unknown).
//   3. root → /root.
//   4. Windows hosts (platform = "windows") get the Windows-shaped
//      set; no Unix paths leak in.

function host(overrides: Partial<{ os: string; platform: string; current_user: string }>) {
    return {
        id: "h1",
        project_id: "p1",
        fingerprint: "fp",
        fingerprint_fallback: false,
        first_seen_at: "",
        last_seen_at: "",
        ...overrides,
    } as Parameters<typeof quickPathsForHost>[0];
}

function pathsByLabel(out: QuickPath[] | null): Record<string, string> {
    const map: Record<string, string> = {};
    for (const p of out ?? []) map[p.label] = p.path;
    return map;
}

describe("quickPathsForHost", () => {
    it("returns Unix shortcuts for a Linux host with a non-root user", () => {
        const out = quickPathsForHost(host({ platform: "ubuntu", current_user: "alice" }));
        const m = pathsByLabel(out);
        expect(m["/"]).toBe("/");
        expect(m["~"]).toBe("/home/alice");
        expect(m["/etc"]).toBe("/etc");
        expect(m["/var"]).toBe("/var");
        expect(m["/tmp"]).toBe("/tmp");
    });

    it("uses /root for the root user", () => {
        const out = quickPathsForHost(host({ platform: "ubuntu", current_user: "root" }));
        expect(pathsByLabel(out)["~"]).toBe("/root");
    });

    it("omits the ~ chip when no current_user is reported", () => {
        const out = quickPathsForHost(host({ platform: "ubuntu" }));
        const m = pathsByLabel(out);
        expect(m["~"]).toBeUndefined();
        // The other chips are still there.
        expect(m["/"]).toBe("/");
    });

    it("returns Windows-shaped shortcuts for Windows hosts", () => {
        const out = quickPathsForHost(
            host({ os: "windows", platform: "windows", current_user: "Alice" }),
        );
        const m = pathsByLabel(out);
        expect(m["C:\\"]).toBe("C:\\");
        // No Unix roots.
        expect(m["/"]).toBeUndefined();
        expect(m["/etc"]).toBeUndefined();
        // Home expands under C:\Users\<user>.
        expect(m["~"]).toBe("C:\\Users\\Alice");
    });

    it("falls back to the Unix set when the host has no platform info at all", () => {
        const out = quickPathsForHost(host({}));
        const m = pathsByLabel(out);
        // Conservative default — Linux is by far the most common
        // agent platform; an unclassified host gets the Unix set.
        expect(m["/"]).toBe("/");
    });

    it("returns null when the host itself is null (e.g. still loading)", () => {
        // The chip row consumer treats null as 'not yet ready' and
        // hides while waiting on the host fetch.
        expect(quickPathsForHost(null)).toBeNull();
    });
});
