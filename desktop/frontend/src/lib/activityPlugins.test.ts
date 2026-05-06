import { describe, expect, it } from "vitest";

import { missingFor } from "./activityPlugins";

// missingFor is the small-but-load-bearing helper that drives per-
// tab plugin gating. Q5 rewired it from a hardcoded REQUIRED_PLUGINS
// map to the PLUGIN_UI_REGISTRY's per-entry requiredPluginIDs, so
// the tests pin two things:
//   1. Honesty about inputs — null installed = "still loading", no
//      install guide; missing required ids surface verbatim.
//   2. OS-aware lookup — Processes ships per-OS variants
//      (sys-procs-{linux,darwin,windows}); the right one is picked
//      by hostOS so a darwin host doesn't get told to install the
//      Linux variant.

describe("missingFor", () => {
    it("returns [] when the installed set is null (still loading)", () => {
        expect(missingFor("files", null, "linux")).toEqual([]);
    });

    it("returns [] for activities not in the registry", () => {
        // The hardcoded "plugins" tab has no registry entry; gating
        // doesn't apply.
        expect(missingFor("plugins", new Set(), "linux")).toEqual([]);
        expect(missingFor("nonsense-key", new Set(), "linux")).toEqual([]);
    });

    it("returns required minus installed for Files (alwaysVisible entry)", () => {
        // Files declares requiredPluginIDs = [sys-files-read, sys-files-write].
        const got = missingFor(
            "files",
            new Set(["com.platypus.sys-files-read"]),
            "linux",
        );
        expect(got).toEqual(["com.platypus.sys-files-write"]);
    });

    it("returns [] when every required plugin is installed", () => {
        const installed = new Set([
            "com.platypus.sys-files-read",
            "com.platypus.sys-files-write",
        ]);
        expect(missingFor("files", installed, "linux")).toEqual([]);
    });

    it("Processes lookup is OS-aware — darwin doesn't get sys-procs-linux", () => {
        // On darwin, the matching Processes entry requires
        // sys-procs-darwin + sys-process. Linux's sys-procs-linux is
        // irrelevant.
        const installed = new Set(["com.platypus.sys-procs-darwin"]);
        expect(missingFor("processes", installed, "darwin")).toEqual([
            "com.platypus.sys-process",
        ]);
        expect(missingFor("processes", installed, "linux")).toEqual([
            "com.platypus.sys-procs-linux",
            "com.platypus.sys-process",
        ]);
    });
});
