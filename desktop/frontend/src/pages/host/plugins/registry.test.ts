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
    entryActivityKey,
    entryMissingPluginIDs,
    entryReady,
    entryRequiredPluginIDs,
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
    return entries.filter((entry) => {
        // OS gate.
        if (entry.osTargets && entry.osTargets.length > 0) {
            if (hostOS !== "" && !entry.osTargets.includes(hostOS)) {
                return false;
            }
        }
        // Install gate (skip when alwaysVisible).
        if (!entry.alwaysVisible) {
            if (!installed || !installed.has(entry.pluginID)) return false;
        }
        return true;
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

    describe("visiblePluginEntries (filter logic, install-gated default)", () => {
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

    describe("alwaysVisible — entries skip the install gate", () => {
        const ENTRIES_AV: PluginUIEntry[] = [
            {
                pluginID: "com.example.always-visible",
                title: "Always Visible",
                icon: Cog,
                alwaysVisible: true,
                component: NoopComponent,
            },
            {
                pluginID: "com.example.install-only",
                title: "Install Only",
                icon: Cog,
                component: NoopComponent,
            },
        ];

        it("alwaysVisible entry appears even with empty installed set", () => {
            const got = filter(ENTRIES_AV, null, "linux");
            expect(got.map((e) => e.pluginID)).toEqual([
                "com.example.always-visible",
            ]);
        });

        it("alwaysVisible entry appears regardless of install status", () => {
            // Neither installed.
            expect(filter(ENTRIES_AV, new Set(), "linux").map((e) => e.pluginID))
                .toEqual(["com.example.always-visible"]);
            // Both installed.
            expect(
                filter(
                    ENTRIES_AV,
                    new Set([
                        "com.example.always-visible",
                        "com.example.install-only",
                    ]),
                    "linux",
                ).map((e) => e.pluginID),
            ).toEqual([
                "com.example.always-visible",
                "com.example.install-only",
            ]);
        });

        it("OS gate still applies to alwaysVisible entries", () => {
            const ENTRIES = [
                {
                    pluginID: "com.example.av-linux",
                    title: "AV Linux",
                    icon: Cog,
                    alwaysVisible: true,
                    osTargets: ["linux"],
                    component: NoopComponent,
                } satisfies PluginUIEntry,
            ];
            // On darwin: hidden even though alwaysVisible.
            expect(filter(ENTRIES, null, "darwin")).toEqual([]);
            // On linux: visible.
            expect(filter(ENTRIES, null, "linux")).toHaveLength(1);
        });
    });

    describe("entry helpers", () => {
        it("entryRequiredPluginIDs defaults to [pluginID]", () => {
            const e: PluginUIEntry = {
                pluginID: "com.example.x",
                title: "X",
                icon: Cog,
                component: NoopComponent,
            };
            expect(entryRequiredPluginIDs(e)).toEqual(["com.example.x"]);
        });

        it("entryRequiredPluginIDs returns the explicit list when set", () => {
            const e: PluginUIEntry = {
                pluginID: "com.example.files",
                requiredPluginIDs: [
                    "com.example.files-read",
                    "com.example.files-write",
                ],
                title: "Files",
                icon: Cog,
                component: NoopComponent,
            };
            expect(entryRequiredPluginIDs(e)).toEqual([
                "com.example.files-read",
                "com.example.files-write",
            ]);
        });

        it("entryActivityKey defaults to pluginActivityKey(pluginID)", () => {
            const e: PluginUIEntry = {
                pluginID: "com.platypus.sys-pkg-linux",
                title: "Pkg",
                icon: Cog,
                component: NoopComponent,
            };
            expect(entryActivityKey(e)).toBe(
                "plugin:com.platypus.sys-pkg-linux",
            );
        });

        it("entryActivityKey honours an explicit override", () => {
            const e: PluginUIEntry = {
                pluginID: "com.platypus.sys-files-read",
                activityKey: "files",
                title: "Files",
                icon: Cog,
                component: NoopComponent,
            };
            expect(entryActivityKey(e)).toBe("files");
        });

        it("entryReady requires every requiredPluginID to be installed", () => {
            const e: PluginUIEntry = {
                pluginID: "com.example.files",
                requiredPluginIDs: ["a", "b"],
                title: "Files",
                icon: Cog,
                component: NoopComponent,
            };
            expect(entryReady(e, new Set(["a"]))).toBe(false);
            expect(entryReady(e, new Set(["a", "b"]))).toBe(true);
        });

        it("entryMissingPluginIDs lists the unmet requirements", () => {
            const e: PluginUIEntry = {
                pluginID: "com.example.files",
                requiredPluginIDs: ["a", "b", "c"],
                title: "Files",
                icon: Cog,
                component: NoopComponent,
            };
            expect(entryMissingPluginIDs(e, new Set(["a"]))).toEqual([
                "b",
                "c",
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

        it("returns only alwaysVisible entries when nothing in installed set matches install-gated entries", () => {
            const got = visiblePluginEntries(
                new Set(["com.example.fictional"]),
                "linux",
            );
            // Q2: Files + Info are alwaysVisible: true so they appear
            // even with no install matches; install-gated entries
            // (sys-systemd-linux etc.) are absent.
            expect(got.every((e) => e.alwaysVisible === true)).toBe(true);
            expect(got.map((e) => e.pluginID)).toEqual(
                expect.arrayContaining([
                    "com.platypus.sys-files-read",
                    "com.platypus.sys-info",
                ]),
            );
            expect(got.map((e) => e.pluginID)).not.toContain(
                "com.platypus.sys-systemd-linux",
            );
        });

        it("Files + Info entries declare stable activityKey URL slugs", () => {
            // The Q2 migration moved Files / Info into the registry,
            // but their URL slugs ("files" / "info") must stay stable
            // so existing bookmarks and docs keep resolving.
            const filesEntry = PLUGIN_UI_REGISTRY.find(
                (e) => e.pluginID === "com.platypus.sys-files-read",
            );
            const infoEntry = PLUGIN_UI_REGISTRY.find(
                (e) => e.pluginID === "com.platypus.sys-info",
            );
            expect(filesEntry?.activityKey).toBe("files");
            expect(filesEntry?.alwaysVisible).toBe(true);
            expect(filesEntry?.requiredPluginIDs).toEqual([
                "com.platypus.sys-files-read",
                "com.platypus.sys-files-write",
            ]);
            expect(infoEntry?.activityKey).toBe("info");
            expect(infoEntry?.alwaysVisible).toBe(true);
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
