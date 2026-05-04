import { describe, expect, it } from "vitest";

import {
    REQUIRED_PLUGINS,
    activitiesNeedingInstall,
    missingFor,
} from "./activityPlugins";

// activityPlugins is the small-but-load-bearing mapping that drives
// the per-tab plugin gating in HostView. The tests pin three things:
//   1. `missingFor` is honest about its inputs — null installed set
//      means "still loading", which renders no install guide.
//   2. `activitiesNeedingInstall` reflects exactly the activities
//      whose required plugins aren't all installed.
//   3. The mapping itself only references plugins that actually
//      exist in the system catalog (sanity check against typos in
//      REQUIRED_PLUGINS).

describe("missingFor", () => {
    it("returns [] for activities with no required plugins", () => {
        // info has no entry; same with the activity-bar-only "plugins" tab.
        expect(missingFor("info", new Set())).toEqual([]);
        expect(missingFor("plugins", new Set())).toEqual([]);
    });

    it("returns [] when the installed set is null (still loading)", () => {
        // Loading state: tab body's own loader paints during this
        // window. The install guide must not flash spuriously.
        expect(missingFor("files", null)).toEqual([]);
    });

    it("returns the required plugins minus the installed ones", () => {
        const got = missingFor(
            "files",
            new Set(["com.platypus.sys-listdir"]),
        );
        // sys-listdir installed; sys-file-read + sys-fs-write missing.
        expect(got).toEqual([
            "com.platypus.sys-file-read",
            "com.platypus.sys-fs-write",
        ]);
    });

    it("returns [] when every required plugin is installed", () => {
        const installed = new Set([
            "com.platypus.sys-listdir",
            "com.platypus.sys-file-read",
            "com.platypus.sys-fs-write",
        ]);
        expect(missingFor("files", installed)).toEqual([]);
    });
});

describe("activitiesNeedingInstall", () => {
    it("returns empty when nothing is registered yet (loading)", () => {
        expect(activitiesNeedingInstall(null)).toEqual({});
    });

    it("flags activities whose required plugins aren't installed", () => {
        // Only sys-procs installed → processes still missing
        // sys-process-open; sessions also needs sys-process-open;
        // every other gated activity has all plugins missing.
        const got = activitiesNeedingInstall(
            new Set(["com.platypus.sys-procs"]),
        );
        expect(got.files).toBe(true);
        expect(got.sessions).toBe(true);
        expect(got.processes).toBe(true);
        expect(got.security).toBe(true);
        expect(got.config).toBe(true);
        expect(got.tunnels).toBe(true);
        // info / plugins are intentionally not in REQUIRED_PLUGINS,
        // so they never appear here.
        expect(got.info).toBeUndefined();
        expect(got.plugins).toBeUndefined();
    });

    it("returns empty object when every required plugin is installed", () => {
        const installed = new Set<string>();
        for (const list of Object.values(REQUIRED_PLUGINS)) {
            for (const id of list ?? []) installed.add(id);
        }
        expect(activitiesNeedingInstall(installed)).toEqual({});
    });
});

describe("REQUIRED_PLUGINS", () => {
    it("keys are subset of declared activity names", () => {
        // Cheap typo guard — REQUIRED_PLUGINS uses Activity as its
        // index type, so a bad key would be a TypeScript error
        // already, but the test makes the contract explicit.
        const validActivities = new Set([
            "files",
            "info",
            "sessions",
            "processes",
            "security",
            "config",
            "tunnels",
            "plugins",
        ]);
        for (const k of Object.keys(REQUIRED_PLUGINS)) {
            expect(validActivities.has(k)).toBe(true);
        }
    });

    it("every required id matches the system catalog naming convention", () => {
        // Sanity check: avoid "sys-listdir" vs "com.platypus.sys-listdir"
        // typos that would silently break the gate.
        for (const list of Object.values(REQUIRED_PLUGINS)) {
            for (const id of list ?? []) {
                expect(id).toMatch(/^com\.platypus\.sys-/);
            }
        }
    });
});
