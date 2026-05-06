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
//   · Tunnels — sys-tunnel-pull is the agent-side stream owner.
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
    tunnels: ["com.platypus.sys-tunnel-pull"],
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
