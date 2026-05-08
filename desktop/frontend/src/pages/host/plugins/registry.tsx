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
    AppWindow,
    Cog,
    File,
    HardDrive,
    Info,
    KeyRound,
    Network as NetworkIcon,
    Package as PackageIcon,
    Plug,
    ScrollText,
    ShieldCheck,
    Wrench,
} from "lucide-react";

import ConfigActivity from "./builtin-config/ConfigActivity";
import FilesActivity from "./builtin-files/FilesActivity";
import InfoActivity from "./builtin-info/InfoActivity";
import ProcessesActivity from "./builtin-processes/ProcessesActivity";
import SecurityActivity from "./builtin-security/SecurityActivity";
import SessionsActivity from "./builtin-sessions/SessionsActivity";
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
    /**
     * Reverse-DNS plugin id this entry's "primary" association.
     * Drives the "newly installed" dot indicator and the default
     * activity URL slug.
     */
    pluginID: string;
    /**
     * URL activity slug. Optional; defaults to
     * `pluginActivityKey(pluginID)`. Set explicitly when migrating a
     * legacy hardcoded tab so the URL stays stable
     * (e.g. "files" rather than "plugin:com.platypus.sys-files-read"
     * — old bookmarks keep working).
     */
    activityKey?: string;
    /**
     * Plugin ids that must ALL be installed for this view to
     * function. Defaults to [pluginID]. Use cases:
     *   - Files needs sys-files-read AND sys-files-write
     *   - Processes needs sys-procs-linux AND sys-process
     * Used by the activity-bar's dimming logic + the install-guide
     * the entry surfaces when not all required plugins are present.
     */
    requiredPluginIDs?: ReadonlyArray<string>;
    /**
     * When true, the icon stays in the sidebar even when required
     * plugins aren't installed; clicking shows an install guide
     * instead of the component (so the operator can discover the
     * capability without browsing the Plugins tab first).
     *
     * Default false ≡ "only-after-install" — the icon doesn't
     * appear until the plugin is installed. Recommended for niche
     * plugins (sys-pkg, sys-journald) where always showing a
     * dimmed icon would clutter the bar.
     *
     * High-baseline plugins (Files, Info, Sessions, Processes,
     * Security, Config, Tunnels) opt in with alwaysVisible: true
     * so a fresh-install agent still shows the discoverable set.
     */
    alwaysVisible?: boolean;
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

/** Resolves the required-plugin list, using the pluginID default. */
export function entryRequiredPluginIDs(
    entry: PluginUIEntry,
): ReadonlyArray<string> {
    return entry.requiredPluginIDs ?? [entry.pluginID];
}

/** Resolves the URL activity slug for an entry. */
export function entryActivityKey(entry: PluginUIEntry): string {
    return entry.activityKey ?? pluginActivityKey(entry.pluginID);
}

/** All required plugins installed? */
export function entryReady(
    entry: PluginUIEntry,
    installed: ReadonlySet<string>,
): boolean {
    for (const id of entryRequiredPluginIDs(entry)) {
        if (!installed.has(id)) return false;
    }
    return true;
}

/** Required plugin ids that haven't been installed yet. */
export function entryMissingPluginIDs(
    entry: PluginUIEntry,
    installed: ReadonlySet<string>,
): string[] {
    return entryRequiredPluginIDs(entry).filter((id) => !installed.has(id));
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
    // ---- Built-in tabs migrated from FIRST_PARTY_ACTIVITIES (Q2). ----
    // These keep their stable URL slugs ("files", "info") via the
    // entry.activityKey override so existing bookmarks / docs keep
    // resolving. alwaysVisible: true mirrors the legacy "icon always
    // present" UX; clicking when a required plugin is missing routes
    // through <RequiresPlugins> to the install guide.
    {
        pluginID: "com.platypus.sys-files-read",
        activityKey: "files",
        requiredPluginIDs: [
            "com.platypus.sys-files-read",
            "com.platypus.sys-files-write",
        ],
        alwaysVisible: true,
        title: "Files",
        icon: File,
        component: FilesActivity,
    },
    {
        pluginID: "com.platypus.sys-info",
        activityKey: "info",
        // sys-info is mandatory core (always installed), but list it
        // here so the entryReady check stays meaningful if an operator
        // ever uninstalls it manually.
        requiredPluginIDs: ["com.platypus.sys-info"],
        alwaysVisible: true,
        title: "Info",
        icon: Info,
        component: InfoActivity,
    },
    // ---- Sessions / Processes migrated from FIRST_PARTY in Q3. ----
    // Sessions is OS-agnostic (sys-process is one plugin everywhere);
    // Processes is per-OS because the underlying process-list plugin
    // is OS-specific (sys-procs-linux / -darwin / -windows). Each
    // per-OS entry shares the same activityKey ("processes") + the
    // same React component — osTargets + the install gate ensure
    // only one is in `pluginEntries` for any given host.
    {
        pluginID: "com.platypus.sys-process",
        activityKey: "sessions",
        requiredPluginIDs: ["com.platypus.sys-process"],
        alwaysVisible: true,
        title: "Sessions",
        icon: Plug,
        component: SessionsActivity,
    },
    {
        pluginID: "com.platypus.sys-procs-linux",
        activityKey: "processes",
        requiredPluginIDs: [
            "com.platypus.sys-procs-linux",
            "com.platypus.sys-process",
        ],
        alwaysVisible: true,
        osTargets: ["linux"],
        title: "Processes",
        icon: AppWindow,
        component: ProcessesActivity,
    },
    {
        pluginID: "com.platypus.sys-procs-darwin",
        activityKey: "processes",
        requiredPluginIDs: [
            "com.platypus.sys-procs-darwin",
            "com.platypus.sys-process",
        ],
        alwaysVisible: true,
        osTargets: ["darwin"],
        title: "Processes",
        icon: AppWindow,
        component: ProcessesActivity,
    },
    {
        pluginID: "com.platypus.sys-procs-windows",
        activityKey: "processes",
        requiredPluginIDs: [
            "com.platypus.sys-procs-windows",
            "com.platypus.sys-process",
        ],
        alwaysVisible: true,
        osTargets: ["windows"],
        title: "Processes",
        icon: AppWindow,
        component: ProcessesActivity,
    },
    // ---- Security / Config migrated from FIRST_PARTY (Q4). ----
    {
        pluginID: "com.platypus.sys-security",
        activityKey: "security",
        requiredPluginIDs: ["com.platypus.sys-security"],
        alwaysVisible: true,
        title: "Security",
        icon: ShieldCheck,
        component: SecurityActivity,
    },
    {
        pluginID: "com.platypus.sys-config-audit",
        activityKey: "config",
        requiredPluginIDs: ["com.platypus.sys-config-audit"],
        alwaysVisible: true,
        title: "Config",
        icon: KeyRound,
        component: ConfigActivity,
    },

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
 * Resolves the registry entry that owns an activity slug for the
 * given host OS. Used by RequiresPlugins to derive the install gate
 * (entry.requiredPluginIDs) without each call site duplicating the
 * activity → plugin id mapping. Returns null when the activity slug
 * isn't registered (e.g. the "plugins" catalogue tab, which is
 * still hardcoded).
 *
 * Multi-entry activities (Processes ships per-OS variants sharing
 * activityKey="processes") are disambiguated by `hostOS`: the entry
 * whose osTargets includes hostOS wins. Empty / unknown hostOS
 * picks the first registered entry — better than returning null,
 * which would skip the install gate entirely.
 */
export function entryForActivity(
    activityKey: string,
    hostOS: string,
): PluginUIEntry | null {
    const matches = PLUGIN_UI_REGISTRY.filter(
        (e) => entryActivityKey(e) === activityKey,
    );
    if (matches.length === 0) return null;
    if (matches.length === 1) return matches[0]!;
    if (hostOS !== "") {
        const osMatch = matches.find(
            (e) => e.osTargets && e.osTargets.includes(hostOS),
        );
        if (osMatch) return osMatch;
    }
    return matches[0]!;
}

/**
 * visiblePluginEntries returns the registry entries that should
 * appear in the activity bar for the given host. The rule has two
 * layers:
 *
 *   - OS gate: an entry whose os_targets is non-empty AND doesn't
 *     include host.os is hidden (regardless of alwaysVisible).
 *     A linux-only icon never shows up on a darwin host.
 *
 *   - Install gate (only when entry.alwaysVisible is NOT true):
 *     entry.pluginID must be in `installed`. alwaysVisible: true
 *     entries skip this gate so the operator sees the icon (dimmed,
 *     with an install-guide on click) even before the plugin is
 *     installed — the discovery affordance for high-baseline plugins.
 *
 * Empty installed (loading / no plugins yet) is still respected as
 * "the install set isn't known yet" — alwaysVisible entries appear,
 * install-gated entries don't.
 */
export function visiblePluginEntries(
    installed: ReadonlySet<string> | null | undefined,
    hostOS: string,
): PluginUIEntry[] {
    return PLUGIN_UI_REGISTRY.filter((entry) => {
        // OS gate.
        if (entry.osTargets && entry.osTargets.length > 0) {
            // Empty hostOS ≡ "OS unknown" → don't filter (better
            // visible-too-much than silently hidden for a freshly-
            // enrolled agent that hasn't reported sysinfo yet).
            if (hostOS !== "" && !entry.osTargets.includes(hostOS)) {
                return false;
            }
        }
        // Install gate (only-after-install entries).
        if (!entry.alwaysVisible) {
            if (!installed || !installed.has(entry.pluginID)) return false;
        }
        return true;
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
