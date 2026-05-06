// Per-host install-status hooks + the missingFor helper that drives
// per-tab plugin gating in HostView.
//
// Wire shape: every tab body wraps in <RequiresPlugins activity={...}>.
// The component reads the host's installed plugin list (cached),
// looks up the activity's PLUGIN_UI_REGISTRY entry to learn its
// requiredPluginIDs (per-OS-aware), and either renders children
// (all installed) or an InstallGuide that points the operator at
// the Plugins tab to add the missing pieces.
//
// Q5 deleted the standalone REQUIRED_PLUGINS map — the registry's
// per-entry requiredPluginIDs is the single source of truth now.

import { useQuery } from "@tanstack/react-query";

import { listPlugins } from "./api/agents/plugins";
import {
    entryForActivity,
    entryMissingPluginIDs,
} from "../pages/host/plugins/registry";

// useInstalledPluginIDs returns the set of installed plugin ids on
// the agent. Single shared query-key so every tab reads the same
// cached result; mutations from PluginsTab invalidate the same
// key. agentID="" disables the query (host record without an
// agent yet — see the corresponding empty state in PluginsTab).
//
// data is `null` while loading; callers render a neutral state in
// that case so they don't flash the install guide for a host that
// turns out to have everything installed.
export function useInstalledPluginIDs(
    projectID: string,
    agentID: string,
): {
    ids: Set<string> | null;
    isLoading: boolean;
    isError: boolean;
} {
    const q = useQuery({
        queryKey: ["agent-plugins", projectID, agentID],
        queryFn: () => listPlugins(projectID, agentID),
        enabled: agentID !== "",
        refetchOnWindowFocus: false,
        retry: false,
    });
    return {
        ids: q.data ? new Set(q.data.filter((p) => p.enabled).map((p) => p.id)) : null,
        isLoading: q.isLoading,
        isError: q.isError,
    };
}

/**
 * missingFor returns the subset of an activity's required plugins
 * that the operator hasn't installed (or has installed but
 * disabled). Empty list = activity ready to render.
 *
 * `installed === null` surfaces as "all OK, render children" — the
 * loading state gets a neutral flash from the children's own
 * loaders rather than a spurious install guide.
 *
 * Multi-entry activities (Processes ships per-OS variants) are
 * disambiguated via `hostOS`. An empty hostOS picks the first
 * matching entry, which keeps the install guide informative even
 * on hosts that haven't reported sysinfo yet.
 */
export function missingFor(
    activityKey: string,
    installed: Set<string> | null,
    hostOS: string,
): string[] {
    if (installed === null) return [];
    const entry = entryForActivity(activityKey, hostOS);
    if (!entry) return [];
    return entryMissingPluginIDs(entry, installed);
}

// ---------------------------------------------------------------------------
// useNewPluginActivities — track which plugin-shipped activities the
// operator hasn't clicked yet, so the activity bar can render a "new"
// dot on freshly-installed plugin icons.
// ---------------------------------------------------------------------------
//
// Storage model: a per-host Set<plugin_id> in localStorage at
// `seen-plugin-activities:<projectID>:<agentID>`. Membership = the
// operator has clicked the icon for that plugin at least once.
//
// First-encounter bootstrap: if there's no localStorage key at all
// for this host, we seed the set with whatever's currently
// installed. Without this bootstrap every existing plugin would
// look "new" the first time the operator opened the host detail
// page after upgrading — wrong, those aren't actually new to them.
//
// "new" = (installed AND in the registry — i.e. has a UI tab) AND
// not in the seen set. That's the predicate the activity bar
// renders the dot for; clicking the icon calls markSeen which
// removes the dot.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

const SEEN_KEY_PREFIX = "seen-plugin-activities:";

function seenKey(projectID: string, agentID: string): string {
    return `${SEEN_KEY_PREFIX}${projectID}:${agentID}`;
}

function readSeen(projectID: string, agentID: string): Set<string> | null {
    try {
        const raw = localStorage.getItem(seenKey(projectID, agentID));
        if (raw === null) return null;
        const parsed: unknown = JSON.parse(raw);
        if (!Array.isArray(parsed)) return new Set();
        return new Set(parsed.filter((x): x is string => typeof x === "string"));
    } catch {
        return null;
    }
}

function writeSeen(projectID: string, agentID: string, seen: Set<string>): void {
    try {
        localStorage.setItem(
            seenKey(projectID, agentID),
            JSON.stringify(Array.from(seen)),
        );
    } catch {
        // Quota exceeded / private browsing: silently drop. The dot
        // will reappear on next render but the click→mark-seen flow
        // still keeps it gone for the current session.
    }
}

/**
 * Returns the set of "new" plugin activities (installed plugin ids
 * the operator hasn't clicked the sidebar icon for yet) plus a
 * `markSeen(pluginID)` callback the parent invokes from its onSelect
 * handler.
 *
 * Empty installed (loading) → empty Set; the bar just doesn't draw
 * any dots until the data arrives. After bootstrap, only plugins
 * that get installed AFTER the first time the operator visits this
 * host page get a dot.
 */
export function useNewPluginActivities(
    projectID: string,
    agentID: string,
    installedPluginIDs: ReadonlySet<string> | null,
): {
    newPluginIDs: ReadonlySet<string>;
    markSeen: (pluginID: string) => void;
} {
    const [seen, setSeen] = useState<Set<string>>(() => new Set());
    // Track whether we've bootstrapped for this (projectID, agentID)
    // so the effect below only seeds once per host visit.
    const bootstrappedRef = useRef<string>("");

    // Bootstrap on first encounter of (projectID, agentID) AFTER
    // installedPluginIDs is non-null. Bootstrapping seeds the seen
    // set with whatever's already installed when the localStorage
    // key didn't exist before — preventing the "every plugin gets
    // a dot on first visit" surprise.
    useEffect(() => {
        if (!projectID || !agentID || !installedPluginIDs) return;
        const hostKey = `${projectID}:${agentID}`;
        if (bootstrappedRef.current === hostKey) return;

        const stored = readSeen(projectID, agentID);
        if (stored !== null) {
            setSeen(stored);
            bootstrappedRef.current = hostKey;
            return;
        }
        // No prior entry → seed with the current installed set.
        const seed = new Set(installedPluginIDs);
        writeSeen(projectID, agentID, seed);
        setSeen(seed);
        bootstrappedRef.current = hostKey;
    }, [projectID, agentID, installedPluginIDs]);

    const markSeen = useCallback(
        (pluginID: string) => {
            if (!projectID || !agentID) return;
            setSeen((prev) => {
                if (prev.has(pluginID)) return prev;
                const next = new Set(prev);
                next.add(pluginID);
                writeSeen(projectID, agentID, next);
                return next;
            });
        },
        [projectID, agentID],
    );

    const newPluginIDs = useMemo(() => {
        if (!installedPluginIDs) return new Set<string>();
        const out = new Set<string>();
        for (const id of installedPluginIDs) {
            if (!seen.has(id)) out.add(id);
        }
        return out;
    }, [installedPluginIDs, seen]);

    return { newPluginIDs, markSeen };
}

// Exposed for tests so they can clear the localStorage layer
// without poking at the constants directly.
export const _seenKey = seenKey;
