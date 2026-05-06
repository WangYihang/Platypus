// Per-activity plugin requirements + the helper hook + wrapper that
// enforces them. The mapping is the single source of truth for
// "which capability lives in which plugin": one place to look at
// when an activity tab is added or a plugin id changes.
//
// Wire shape: every tab body wraps in <RequiresPlugins activity={...}>.
// The component reads the host's installed plugin list (cached),
// compares against REQUIRED_PLUGINS[activity], and either renders
// children (all installed) or an InstallGuide that points the
// operator at the Plugins tab to add the missing pieces.
//
// The activity icon greying lives on top of the same hook in
// ActivityBar — we expose missingPlugins() so the bar can dim
// icons whose tab would land on the install guide.

import { useQuery } from "@tanstack/react-query";

import { listPlugins } from "./api/agents/plugins";

import type { Activity } from "../pages/host/ActivityBar";

// REQUIRED_PLUGINS lists the system-plugin ids each activity tab
// needs to be useful. "Useful" = the primary affordance works:
//   · Files — needs sys-files-read (list_dir + stat + read +
//     scan + archive) AND sys-files-write (mkdir / chmod / delete /
//     rename + write stream). The two were 6 separate plugins
//     before the merge; collapsing fs.read and fs.write each into
//     one plugin halves the operator's "Install" buttons here.
//   · Info / Hardware — sys-info paints the overview cards
//     (including hostname; sys-hostname was folded into sys-info).
//     We don't list sys-info as required here because
//     mandatoryCorePluginIDs guarantees it on every boot.
//   · Sessions — terminal sessions need sys-process (was
//     sys-process-open before merging with sys-exec).
//   · Processes — needs sys-procs-linux (per-OS process list
//     plugin; M1a/M1b will add sys-procs-darwin / -windows) AND
//     sys-process (open a shell from a process row).
//   · Security — sys-security drives the security scan UI.
//   · Config — sys-config-audit drives the config audit UI.
//   · Tunnels — sys-tunnel-tcp is the agent-side stream owner
//     (renamed from sys-tunnel-pull in Sprint 1's I3a).
//   · Plugins — meta tab; needs nothing.
//
// Plugins outside the operator's allowlist surface as "Install"
// prompts; once installed, the tab activates without any further
// state plumbing because the per-tab queries get their normal
// 200 from the agent.
export const REQUIRED_PLUGINS: Partial<Record<Activity, readonly string[]>> = {
    files: [
        "com.platypus.sys-files-read",
        "com.platypus.sys-files-write",
    ],
    sessions: ["com.platypus.sys-process"],
    processes: [
        // TODO: when M1a/M1b ship sys-procs-darwin / -windows, swap
        // this hardcoded ID for an OS-aware lookup so non-linux
        // agents don't flash an "install sys-procs-linux" prompt.
        "com.platypus.sys-procs-linux",
        "com.platypus.sys-process",
    ],
    security: ["com.platypus.sys-security"],
    config: ["com.platypus.sys-config-audit"],
    tunnels: ["com.platypus.sys-tunnel-tcp"],
    // info + plugins intentionally absent — info needs only
    // sys-info (mandatory core, always present); plugins is the
    // catalogue tab and would create a recursive prompt.
};

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

// missingFor returns the subset of REQUIRED_PLUGINS[activity] that
// the operator hasn't installed (or has installed but disabled).
// Empty list = activity ready to render. installed=null surfaces
// as "all OK, render children" — the loading state gets a neutral
// flash from the children's own loaders rather than a spurious
// install guide.
export function missingFor(activity: Activity, installed: Set<string> | null): string[] {
    const required = REQUIRED_PLUGINS[activity];
    if (!required || required.length === 0) return [];
    if (installed === null) return [];
    return required.filter((id) => !installed.has(id));
}

// activitiesNeedingInstall returns a map { activity: true } for
// every activity whose required plugins aren't all installed.
// Used by ActivityBar to dim icons + paint the "needs plugin" dot
// without each tab needing to compute it separately.
export function activitiesNeedingInstall(installed: Set<string> | null): Partial<Record<Activity, boolean>> {
    if (installed === null) return {};
    const out: Partial<Record<Activity, boolean>> = {};
    for (const activity of Object.keys(REQUIRED_PLUGINS) as Activity[]) {
        if (missingFor(activity, installed).length > 0) {
            out[activity] = true;
        }
    }
    return out;
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
