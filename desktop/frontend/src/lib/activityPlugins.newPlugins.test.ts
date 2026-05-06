// Spec for useNewPluginActivities + activitiesNeedingInstall.
//
// useNewPluginActivities is the localStorage-backed "which plugin
// icons should show a 'new' dot?" hook. Coverage:
//   - first encounter (no localStorage entry) seeds with currently-
//     installed → new set is empty
//   - subsequent loads with a fresh plugin id → that id is "new"
//   - markSeen removes from "new" + persists to localStorage
//   - per-host scoping: different (projectID, agentID) → independent state

import { describe, expect, it, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

import { useNewPluginActivities, _seenKey } from "./activityPlugins";

beforeEach(() => {
    localStorage.clear();
});

describe("useNewPluginActivities", () => {
    it("first encounter seeds with currently-installed → no plugin is 'new'", async () => {
        const { result } = renderHook(() =>
            useNewPluginActivities(
                "p1",
                "a1",
                new Set(["com.platypus.sys-systemd-linux"]),
            ),
        );
        await waitFor(() => {
            expect(localStorage.getItem(_seenKey("p1", "a1"))).not.toBeNull();
        });
        // Sys-systemd was already installed when the hook first
        // bootstrapped → no "new" dot.
        expect(Array.from(result.current.newPluginIDs)).toEqual([]);
    });

    it("plugin installed AFTER bootstrap → 'new' until markSeen", async () => {
        // Pre-seed localStorage with one already-seen plugin.
        localStorage.setItem(
            _seenKey("p1", "a1"),
            JSON.stringify(["com.platypus.sys-systemd-linux"]),
        );

        const { result, rerender } = renderHook(
            ({ installed }: { installed: Set<string> }) =>
                useNewPluginActivities("p1", "a1", installed),
            {
                initialProps: {
                    installed: new Set(["com.platypus.sys-systemd-linux"]),
                },
            },
        );

        // Initial: no new plugins (the only installed one is in seen set).
        await waitFor(() => {
            expect(Array.from(result.current.newPluginIDs)).toEqual([]);
        });

        // Operator installs a new plugin → re-render with bigger set.
        rerender({
            installed: new Set([
                "com.platypus.sys-systemd-linux",
                "com.platypus.sys-net-linux",
            ]),
        });
        await waitFor(() => {
            expect(Array.from(result.current.newPluginIDs)).toEqual([
                "com.platypus.sys-net-linux",
            ]);
        });

        // markSeen clears the dot AND persists.
        act(() => {
            result.current.markSeen("com.platypus.sys-net-linux");
        });
        await waitFor(() => {
            expect(Array.from(result.current.newPluginIDs)).toEqual([]);
        });
        const stored = JSON.parse(
            localStorage.getItem(_seenKey("p1", "a1")) ?? "[]",
        );
        expect(stored).toContain("com.platypus.sys-net-linux");
    });

    it("per-host state: separate (projectID, agentID) keys are independent", async () => {
        const { result: r1 } = renderHook(() =>
            useNewPluginActivities("p1", "a1", new Set(["x"])),
        );
        const { result: r2 } = renderHook(() =>
            useNewPluginActivities("p1", "a2", new Set(["x", "y"])),
        );
        await waitFor(() => {
            expect(localStorage.getItem(_seenKey("p1", "a1"))).not.toBeNull();
            expect(localStorage.getItem(_seenKey("p1", "a2"))).not.toBeNull();
        });
        // Both bootstrapped against their respective installed
        // sets → neither sees a "new" plugin.
        expect(Array.from(r1.current.newPluginIDs)).toEqual([]);
        expect(Array.from(r2.current.newPluginIDs)).toEqual([]);
    });

    it("empty installed (loading) → empty new set, no localStorage write", () => {
        const { result } = renderHook(() =>
            useNewPluginActivities("p1", "a1", null),
        );
        expect(Array.from(result.current.newPluginIDs)).toEqual([]);
        expect(localStorage.getItem(_seenKey("p1", "a1"))).toBeNull();
    });

    it("corrupted localStorage value → recovers as empty seen set (don't crash)", async () => {
        localStorage.setItem(_seenKey("p1", "a1"), "not json");
        const { result } = renderHook(() =>
            useNewPluginActivities("p1", "a1", new Set(["x"])),
        );
        // Bootstrap path retried because read returned null on parse
        // failure → set re-seeded with currently-installed.
        await waitFor(() => {
            expect(Array.from(result.current.newPluginIDs)).toEqual([]);
        });
    });
});
