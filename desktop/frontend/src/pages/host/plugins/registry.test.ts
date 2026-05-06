// Spec for the per-plugin UI registry helpers. Component-level
// integration (does the right tab render when the operator clicks
// the right icon?) lives in HostView's higher-level test; here we
// just verify the pure functions: filtering by installed/os, the
// activity-key prefix round-trip, the URL-safe character choice.

import { describe, expect, it } from "vitest";
import { Cog } from "lucide-react";

import {
    PLUGIN_ACTIVITY_PREFIX,
    PLUGIN_UI_REGISTRY,
    parsePluginActivity,
    pluginActivityKey,
    visiblePluginEntries,
    type PluginUIEntry,
} from "./registry";

const NoopComponent = () => null;

const FIXTURES: PluginUIEntry[] = [
    {
        pluginID: "com.example.linux-only",
        title: "Linux only",
        icon: Cog,
        osTargets: ["linux"],
        component: NoopComponent,
    },
    {
        pluginID: "com.example.darwin-only",
        title: "Darwin only",
        icon: Cog,
        osTargets: ["darwin"],
        component: NoopComponent,
    },
    {
        pluginID: "com.example.everywhere",
        title: "Multi-OS",
        icon: Cog,
        component: NoopComponent,
    },
];

// visiblePluginEntries reads from PLUGIN_UI_REGISTRY (real one)
// — to keep these tests pure we pass the FIXTURES array via a
// helper that mirrors visiblePluginEntries' filter logic. The
// real registry is exercised end-to-end in HostView's specs.
function filter(
    entries: PluginUIEntry[],
    installed: ReadonlySet<string> | null,
    hostOS: string,
): PluginUIEntry[] {
    if (!installed) return [];
    return entries.filter((entry) => {
        if (!installed.has(entry.pluginID)) return false;
        if (!entry.osTargets || entry.osTargets.length === 0) return true;
        if (hostOS === "") return true;
        return entry.osTargets.includes(hostOS);
    });
}

describe("registry helpers", () => {
    describe("pluginActivityKey / parsePluginActivity", () => {
        it("encodes a plugin id into a URL-safe activity key", () => {
            expect(pluginActivityKey("com.platypus.sys-systemd-linux")).toBe(
                "plugin:com.platypus.sys-systemd-linux",
            );
        });

        it("uses ':' as separator (so URLs with single :tab segment work)", () => {
            // Plugin ids never contain ':', so this is unambiguous.
            // If we'd used '/', the route /:tab would treat the
            // plugin id as a sub-segment and break.
            expect(PLUGIN_ACTIVITY_PREFIX).toBe("plugin:");
            expect(pluginActivityKey("anything")).not.toContain("/");
        });

        it("round-trips key → id", () => {
            const key = pluginActivityKey("com.platypus.x-y-z");
            expect(parsePluginActivity(key)).toEqual({
                pluginID: "com.platypus.x-y-z",
            });
        });

        it("returns null on a first-party activity name", () => {
            expect(parsePluginActivity("processes")).toBeNull();
            expect(parsePluginActivity("plugins")).toBeNull(); // ≠ "plugin:"
            expect(parsePluginActivity("")).toBeNull();
        });
    });

    describe("visiblePluginEntries (filter logic)", () => {
        it("returns [] when installed set is null (loading)", () => {
            expect(filter(FIXTURES, null, "linux")).toEqual([]);
        });

        it("filters out plugins that aren't installed", () => {
            const installed = new Set(["com.example.linux-only"]);
            const got = filter(FIXTURES, installed, "linux");
            expect(got).toHaveLength(1);
            expect(got[0]?.pluginID).toBe("com.example.linux-only");
        });

        it("hides OS-mismatched plugins", () => {
            // All three installed, but hostOS=darwin → linux-only is hidden.
            const installed = new Set([
                "com.example.linux-only",
                "com.example.darwin-only",
                "com.example.everywhere",
            ]);
            const got = filter(FIXTURES, installed, "darwin");
            const ids = got.map((e) => e.pluginID).sort();
            expect(ids).toEqual([
                "com.example.darwin-only",
                "com.example.everywhere",
            ]);
        });

        it("treats empty osTargets as 'applies everywhere'", () => {
            const installed = new Set(["com.example.everywhere"]);
            expect(filter(FIXTURES, installed, "linux")).toHaveLength(1);
            expect(filter(FIXTURES, installed, "darwin")).toHaveLength(1);
            expect(filter(FIXTURES, installed, "windows")).toHaveLength(1);
        });

        it("treats empty hostOS as 'OS unknown' → don't filter (better visible than hidden)", () => {
            const installed = new Set([
                "com.example.linux-only",
                "com.example.darwin-only",
            ]);
            const got = filter(FIXTURES, installed, "");
            expect(got.map((e) => e.pluginID).sort()).toEqual([
                "com.example.darwin-only",
                "com.example.linux-only",
            ]);
        });
    });

    describe("real PLUGIN_UI_REGISTRY", () => {
        it("filters by host OS via the real visiblePluginEntries", () => {
            // sys-systemd-linux is in the real registry with osTargets=[linux].
            const onLinux = visiblePluginEntries(
                new Set(["com.platypus.sys-systemd-linux"]),
                "linux",
            );
            expect(onLinux.map((e) => e.pluginID)).toContain(
                "com.platypus.sys-systemd-linux",
            );

            const onDarwin = visiblePluginEntries(
                new Set(["com.platypus.sys-systemd-linux"]),
                "darwin",
            );
            expect(onDarwin.map((e) => e.pluginID)).not.toContain(
                "com.platypus.sys-systemd-linux",
            );
        });

        it("returns [] when nothing in installed set matches the registry", () => {
            const got = visiblePluginEntries(
                new Set(["com.example.fictional"]),
                "linux",
            );
            expect(got).toEqual([]);
        });
    });

    // ---- N3 sanity: every M-phase system plugin has a registry entry ----
    describe("registry contents (N3 bulk migration)", () => {
        const expectedIDs = [
            // Services
            "com.platypus.sys-systemd-linux",
            "com.platypus.sys-services-darwin",
            "com.platypus.sys-services-windows",
            // Disks
            "com.platypus.sys-disk-linux",
            "com.platypus.sys-disk-darwin",
            "com.platypus.sys-disk-windows",
            // Network
            "com.platypus.sys-net-linux",
            "com.platypus.sys-net-darwin",
            "com.platypus.sys-net-windows",
            // Packages
            "com.platypus.sys-pkg-linux",
            "com.platypus.sys-pkg-darwin",
            "com.platypus.sys-pkg-windows",
            // Logs
            "com.platypus.sys-journald-linux",
        ];

        it("contains every expected M-phase plugin id", () => {
            const ids = new Set(PLUGIN_UI_REGISTRY.map((e) => e.pluginID));
            for (const id of expectedIDs) {
                expect(ids.has(id), `missing registry entry for ${id}`).toBe(true);
            }
        });

        it("every entry has a non-empty title and component", () => {
            for (const entry of PLUGIN_UI_REGISTRY) {
                expect(entry.title.length, `${entry.pluginID} title`).toBeGreaterThan(0);
                expect(entry.component, `${entry.pluginID} component`).toBeDefined();
            }
        });

        it("OS-suffixed plugin ids declare matching osTargets", () => {
            for (const entry of PLUGIN_UI_REGISTRY) {
                if (entry.pluginID.endsWith("-linux")) {
                    expect(entry.osTargets).toEqual(["linux"]);
                } else if (entry.pluginID.endsWith("-darwin")) {
                    expect(entry.osTargets).toEqual(["darwin"]);
                } else if (entry.pluginID.endsWith("-windows")) {
                    expect(entry.osTargets).toEqual(["windows"]);
                }
            }
        });

        it("plugin ids are unique within the registry", () => {
            const ids = PLUGIN_UI_REGISTRY.map((e) => e.pluginID);
            const set = new Set(ids);
            expect(set.size).toBe(ids.length);
        });
    });
});
