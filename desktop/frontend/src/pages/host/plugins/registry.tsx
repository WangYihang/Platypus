// PLUGIN_UI_REGISTRY — system plugins that surface as a per-host
// activity tab. Each entry maps a plugin id to a React component
// rendered in the host detail pane when the operator clicks the
// corresponding sidebar icon.
//
// Why a TypeScript array (not a YAML DSL):
//   - System plugins ship from this monorepo; FE PRs are part of
//     the plugin's lifecycle anyway. Adding a 30-line wrapper is
//     not a bottleneck.
//   - TypeScript gives end-to-end type safety: the wrapper types
//     its expected RPC response shape, and refactors in the wire
//     contract surface as compile errors here rather than runtime
//     undefined-renders.
//   - Specialised plugins (PTY, tunnel, future chart-shaped) fit
//     naturally as plain React components; no DSL escape hatches.
//
// See plan: /root/.claude/plans/abi-tdd-noble-firefly.md (Sprint 3 /
// N-phase) for the full architectural rationale.

import type { ComponentType } from "react";
import type { LucideProps } from "lucide-react";
import {
    Cog,
    HardDrive,
    Network as NetworkIcon,
    Package as PackageIcon,
    ScrollText,
    Wrench,
} from "lucide-react";

import { SystemdServices } from "./sys-systemd-linux/Services";
import { Filesystems } from "./sys-disk/Filesystems";
import { Network as NetworkTab } from "./sys-net/Network";
import { Packages } from "./sys-pkg/Packages";
import { Services as ServicesTab } from "./sys-services/Services";
import { JournaldLogs } from "./sys-journald-linux/Logs";

/**
 * Props every plugin tab component receives. Plumbed by HostView's
 * activity router; mirrors the existing first-party tab signature
 * (so a plugin tab is interchangeable with a hardcoded one if we
 * ever migrate Sessions / Processes / etc.).
 */
export interface PluginUIProps {
    projectID: string;
    agentID: string;
    hostOS: string;
    /** True while this plugin's tab is the visible activity. Used
     * to gate polling so offscreen tabs don't keep refetching. */
    active: boolean;
}

export interface PluginUIEntry {
    /** Reverse-DNS plugin id matching `installed_plugin.id`. */
    pluginID: string;
    /** Human-readable label for the sidebar tooltip. */
    title: string;
    /** Lucide icon component (rendered inline in the activity bar). */
    icon: ComponentType<LucideProps>;
    /**
     * Agent OSes this plugin makes sense on. When non-empty AND the
     * agent's host.os is non-empty AND not in the list, the entry
     * is hidden. Empty / undefined ≡ "applies everywhere".
     */
    osTargets?: ReadonlyArray<string>;
    component: ComponentType<PluginUIProps>;
}

// Per-family components are shared across per-OS plugin variants
// (sys-disk-linux/-darwin/-windows all wrap the same Filesystems
// component, just with their own pluginID). The closure below adapts
// PluginUIProps → the family component's prop shape.
function withPluginID<TExtraProps extends { pluginID: string }>(
    Component: ComponentType<PluginUIProps & TExtraProps>,
    pluginID: string,
): ComponentType<PluginUIProps> {
    const Adapter = (props: PluginUIProps) => (
        <Component {...(props as PluginUIProps & TExtraProps)} pluginID={pluginID} />
    );
    Adapter.displayName = `withPluginID(${pluginID})`;
    return Adapter;
}

export const PLUGIN_UI_REGISTRY: ReadonlyArray<PluginUIEntry> = [
    // ---- Services (per-OS init system) ----
    {
        pluginID: "com.platypus.sys-systemd-linux",
        title: "Services",
        icon: Cog,
        osTargets: ["linux"],
        component: SystemdServices,
    },
    {
        pluginID: "com.platypus.sys-services-darwin",
        title: "Services",
        icon: Cog,
        osTargets: ["darwin"],
        component: withPluginID(ServicesTab, "com.platypus.sys-services-darwin"),
    },
    {
        pluginID: "com.platypus.sys-services-windows",
        title: "Services",
        icon: Cog,
        osTargets: ["windows"],
        component: withPluginID(ServicesTab, "com.platypus.sys-services-windows"),
    },

    // ---- Disks ----
    {
        pluginID: "com.platypus.sys-disk-linux",
        title: "Disks",
        icon: HardDrive,
        osTargets: ["linux"],
        component: withPluginID(Filesystems, "com.platypus.sys-disk-linux"),
    },
    {
        pluginID: "com.platypus.sys-disk-darwin",
        title: "Disks",
        icon: HardDrive,
        osTargets: ["darwin"],
        component: withPluginID(Filesystems, "com.platypus.sys-disk-darwin"),
    },
    {
        pluginID: "com.platypus.sys-disk-windows",
        title: "Disks",
        icon: HardDrive,
        osTargets: ["windows"],
        component: withPluginID(Filesystems, "com.platypus.sys-disk-windows"),
    },

    // ---- Network (listeners + connections) ----
    {
        pluginID: "com.platypus.sys-net-linux",
        title: "Network",
        icon: NetworkIcon,
        osTargets: ["linux"],
        component: withPluginID(NetworkTab, "com.platypus.sys-net-linux"),
    },
    {
        pluginID: "com.platypus.sys-net-darwin",
        title: "Network",
        icon: NetworkIcon,
        osTargets: ["darwin"],
        component: withPluginID(NetworkTab, "com.platypus.sys-net-darwin"),
    },
    {
        pluginID: "com.platypus.sys-net-windows",
        title: "Network",
        icon: NetworkIcon,
        osTargets: ["windows"],
        component: withPluginID(NetworkTab, "com.platypus.sys-net-windows"),
    },

    // ---- Packages ----
    {
        pluginID: "com.platypus.sys-pkg-linux",
        title: "Packages",
        icon: PackageIcon,
        osTargets: ["linux"],
        component: withPluginID(Packages, "com.platypus.sys-pkg-linux"),
    },
    {
        pluginID: "com.platypus.sys-pkg-darwin",
        title: "Packages",
        icon: PackageIcon,
        osTargets: ["darwin"],
        component: withPluginID(Packages, "com.platypus.sys-pkg-darwin"),
    },
    {
        pluginID: "com.platypus.sys-pkg-windows",
        title: "Packages",
        icon: PackageIcon,
        osTargets: ["windows"],
        component: withPluginID(Packages, "com.platypus.sys-pkg-windows"),
    },

    // ---- Logs (linux-only journald; darwin / windows variants are
    // intentionally not in v1 — log show / Get-WinEvent need their
    // own plugin shape; deferred). ----
    {
        pluginID: "com.platypus.sys-journald-linux",
        title: "Logs",
        icon: ScrollText,
        osTargets: ["linux"],
        component: JournaldLogs,
    },
];

// Wrench is unused by the registry directly today but kept in the
// import set so future entries that want a different "settings"-
// shape icon don't have to re-import it. (No-op compile reference.)
export const _RESERVED_ICONS = { Wrench };

/**
 * visiblePluginEntries returns the registry entries that should
 * appear in the activity bar for the given host: installed AND
 * OS-matched. Empty installed (loading / no plugins yet) → empty.
 */
export function visiblePluginEntries(
    installed: ReadonlySet<string> | null | undefined,
    hostOS: string,
): PluginUIEntry[] {
    if (!installed) return [];
    return PLUGIN_UI_REGISTRY.filter((entry) => {
        if (!installed.has(entry.pluginID)) return false;
        if (!entry.osTargets || entry.osTargets.length === 0) return true;
        if (hostOS === "") return true; // unknown OS → don't filter
        return entry.osTargets.includes(hostOS);
    });
}

/**
 * Encode a plugin id into the activity-key form used by the URL.
 *
 * The route table mounts host detail at `/hosts/:hostId/:tab` — a
 * single path segment. We can't put `plugin/<id>` in there without
 * URL-encoding the slash; using `:` as the separator keeps the URL
 * readable since plugin ids never contain `:`. Resulting URLs look
 * like `/hosts/<id>/plugin:com.platypus.sys-systemd-linux`.
 */
export const PLUGIN_ACTIVITY_PREFIX = "plugin:" as const;

export function pluginActivityKey(pluginID: string): string {
    return `${PLUGIN_ACTIVITY_PREFIX}${pluginID}`;
}

/** Parse a plugin activity key back into its plugin id. Returns
 * null when the input isn't a plugin activity (i.e. it's one of
 * the hardcoded first-party activities). */
export function parsePluginActivity(
    activity: string,
): { pluginID: string } | null {
    if (!activity.startsWith(PLUGIN_ACTIVITY_PREFIX)) return null;
    return { pluginID: activity.slice(PLUGIN_ACTIVITY_PREFIX.length) };
}
