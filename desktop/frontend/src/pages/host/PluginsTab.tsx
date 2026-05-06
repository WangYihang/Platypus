import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    Download,
    Loader2,
    MoreHorizontal,
    Plus,
    ScrollText,
    Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

import EmptyState from "../../components/EmptyState";
import { palette, space } from "../../layout/theme";
import { humanizeError } from "../../lib/humanizeError";
import {
    InstalledPlugin,
    enablePlugin,
    installFromSystem,
    listPlugins,
    uninstallPlugin,
} from "../../lib/api/agents/plugins";
import {
    SystemPlugin,
    listSystemPlugins,
} from "../../lib/api/system_plugins";

import PluginLogsDrawer from "./plugins/PluginLogsDrawer";
import InstallFromMarketplaceDialog from "./plugins/InstallFromMarketplaceDialog";

interface Props {
    projectID: string;
    /** Host row UUID — used for cache keys + react-query invalidation. */
    hostID: string;
    /**
     * The host's agent_id (cert SAN). All `/agents/:agent_id/...`
     * endpoints key on this, NOT on the host row UUID. Empty string
     * when the host record exists but the agent has never enrolled
     * (e.g. fingerprint-only fallback rows); the tab renders an
     * empty-state in that case rather than firing a guaranteed-404.
     */
    agentID: string;
    /**
     * Agent's reported runtime.GOOS ("linux", "darwin", "windows", "").
     * Used by the Available section to hide system plugins whose
     * manifest os_targets doesn't include this OS — e.g. a linux
     * agent shouldn't see sys-procs-darwin in its installable list.
     * Empty string ≡ "OS unknown" → don't filter (show everything).
     */
    hostOS: string;
    active: boolean;
}

// PluginsTab is the per-host plugin management surface. It lists what
// the agent reports as installed, lets the operator toggle each one
// enabled/disabled, uninstall (with optional state purge), and tail
// the per-plugin log ring.
//
// Install-from-marketplace lives in a sibling iteration; that flow
// reuses the CapabilityConfirmDialog from components/ and posts to
// the same `/plugins` endpoint this tab reads from.
export default function PluginsTab({
    projectID,
    hostID,
    agentID,
    hostOS,
    active,
}: Props) {
    const queryClient = useQueryClient();

    const queryKey = ["agent-plugins", projectID, hostID] as const;

    const plugins = useQuery({
        queryKey,
        queryFn: () => listPlugins(projectID, agentID),
        // Don't fire when we don't have an agent_id yet — would always
        // 404 ("agent not connected") and the empty-state below covers
        // the "host exists but agent never enrolled" case more cleanly.
        enabled: active && agentID !== "",
        // Plugins state changes only on operator action; no need to
        // poll. Mutations invalidate this key.
        refetchOnWindowFocus: false,
        retry: false,
    });

    // System catalog — server-wide list of every system-eligible
    // plugin staged on this server. Used to render the Available
    // section (everything not yet installed on this host). Server-
    // wide so the cache is shared across hosts; per-host filtering
    // is purely client-side.
    const systemCatalog = useQuery({
        queryKey: ["system-plugins"] as const,
        queryFn: () => listSystemPlugins(),
        enabled: active,
        refetchOnWindowFocus: false,
        retry: false,
    });

    if (agentID === "") {
        return (
            <EmptyState
                title="Agent not enrolled"
                description="This host record exists but no agent has connected yet. Plugins can be managed once the agent's mTLS handshake completes."
            />
        );
    }

    const enable = useMutation({
        mutationFn: (vars: { id: string; enabled: boolean }) =>
            enablePlugin(projectID, agentID, vars.id, vars.enabled),
        onSuccess: () => {
            void queryClient.invalidateQueries({ queryKey });
        },
        onError: (err) => toast.error(humanizeError(err)),
    });

    const [confirming, setConfirming] = useState<InstalledPlugin | null>(null);
    const [purgeState, setPurgeState] = useState(false);

    const uninstall = useMutation({
        mutationFn: (vars: { id: string; purgeState: boolean }) =>
            uninstallPlugin(projectID, agentID, vars.id, { purgeState: vars.purgeState }),
        onSuccess: () => {
            toast.success("Plugin uninstalled");
            setConfirming(null);
            setPurgeState(false);
            void queryClient.invalidateQueries({ queryKey });
        },
        onError: (err) => toast.error(humanizeError(err)),
    });

    const [logsTarget, setLogsTarget] = useState<InstalledPlugin | null>(null);
    const [installerOpen, setInstallerOpen] = useState(false);

    // installFromSystem mutation — separate from the marketplace
    // install path because the trust source (system publisher key)
    // and the artefact store (server-side bundle) are distinct.
    const installSystem = useMutation({
        mutationFn: (vars: { p: SystemPlugin }) =>
            installFromSystem(projectID, agentID, {
                pluginID: vars.p.id,
                version: vars.p.version,
                // Grant the FULL declared capability set the manifest
                // requested. System plugins are server-trusted so we
                // don't make the operator pick a subset here; if they
                // want a narrower grant they can install a custom
                // build of the same plugin via the marketplace path.
                grantedCapabilities: vars.p.capabilities ?? [],
            }),
        onSuccess: (_res, vars) => {
            toast.success(`Installed ${vars.p.name}`);
            void queryClient.invalidateQueries({ queryKey });
        },
        onError: (err) => toast.error(humanizeError(err)),
    });

    // Memoised derivations — declared BEFORE the early-return guards
    // below so React's rules-of-hooks invariant holds across the
    // loading→loaded transition (every render must call the same
    // hooks in the same order).
    const list = plugins.data ?? [];
    const installedIDs = useMemo(
        () => new Set(list.map((p) => p.id)),
        [list],
    );
    // Available = system catalog minus installed minus OS-mismatched.
    // os_targets empty / undefined ≡ "applies everywhere" → always
    // shown. host.os empty ≡ "OS unknown" → don't filter (better to
    // show too much than to silently hide everything for a freshly-
    // enrolled agent that hasn't reported sysinfo yet).
    const available = useMemo<SystemPlugin[]>(() => {
        const catalog = systemCatalog.data ?? [];
        return catalog.filter((p) => {
            if (installedIDs.has(p.id)) return false;
            if (
                hostOS &&
                p.os_targets &&
                p.os_targets.length > 0 &&
                !p.os_targets.includes(hostOS)
            ) {
                return false;
            }
            return true;
        });
    }, [systemCatalog.data, installedIDs, hostOS]);

    if (plugins.isLoading) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: space[6] }}>
                <Loader2 className="size-5 animate-spin" />
            </div>
        );
    }

    if (plugins.error) {
        return (
            <EmptyState
                title="Couldn't load plugins"
                description={humanizeError(plugins.error)}
            />
        );
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <div style={{ display: "flex", justifyContent: "flex-end" }}>
                <Button size="sm" onClick={() => setInstallerOpen(true)}>
                    <Plus className="size-3.5" />
                    Install from Marketplace
                </Button>
            </div>
            {list.length === 0 ? (
                <EmptyState
                    title="No plugins installed"
                    description="Install plugins from the marketplace to give this agent capabilities."
                />
            ) : (
                <section>
                    <SectionHeading>
                        Installed ({list.length})
                    </SectionHeading>
                    <ul
                        style={{
                            listStyle: "none",
                            padding: 0,
                            margin: 0,
                            display: "flex",
                            flexDirection: "column",
                            gap: space[2],
                        }}
                    >
                        {list.map((p) => (
                            <PluginRow
                                key={p.id}
                                plugin={p}
                                onToggle={(next) =>
                                    enable.mutate({ id: p.id, enabled: next })
                                }
                                onUninstall={() => setConfirming(p)}
                                onViewLogs={() => setLogsTarget(p)}
                            />
                        ))}
                    </ul>
                </section>
            )}

            {available.length > 0 && (
                <section>
                    <SectionHeading>
                        Available ({available.length})
                    </SectionHeading>
                    <ul
                        style={{
                            listStyle: "none",
                            padding: 0,
                            margin: 0,
                            display: "flex",
                            flexDirection: "column",
                            gap: space[2],
                        }}
                    >
                        {available.map((p) => (
                            <AvailablePluginRow
                                key={p.id}
                                plugin={p}
                                installing={
                                    installSystem.isPending &&
                                    installSystem.variables?.p.id === p.id
                                }
                                onInstall={() => installSystem.mutate({ p })}
                            />
                        ))}
                    </ul>
                </section>
            )}

            <AlertDialog
                open={confirming !== null}
                onOpenChange={(open) => {
                    if (!open) {
                        setConfirming(null);
                        setPurgeState(false);
                    }
                }}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>
                            Uninstall {confirming?.name}?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                            The plugin will be removed from this agent. Any RPC
                            or stream that depends on it will fail until it's
                            reinstalled.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <div className="flex items-center gap-2 px-1">
                        <Checkbox
                            id="purge-state"
                            checked={purgeState}
                            onCheckedChange={(v) => setPurgeState(v === true)}
                        />
                        <Label htmlFor="purge-state" className="text-sm">
                            Also purge plugin state (host_kv data)
                        </Label>
                    </div>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={() => {
                                if (!confirming) return;
                                uninstall.mutate({
                                    id: confirming.id,
                                    purgeState,
                                });
                            }}
                            disabled={uninstall.isPending}
                        >
                            Uninstall
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>

            <PluginLogsDrawer
                projectID={projectID}
                agentID={agentID}
                plugin={logsTarget}
                onClose={() => setLogsTarget(null)}
            />

            <InstallFromMarketplaceDialog
                open={installerOpen}
                projectID={projectID}
                agentID={agentID}
                onClose={() => setInstallerOpen(false)}
                onInstalled={() => {
                    void queryClient.invalidateQueries({ queryKey });
                }}
            />
        </div>
    );
}

function PluginRow({
    plugin,
    onToggle,
    onUninstall,
    onViewLogs,
}: {
    plugin: InstalledPlugin;
    onToggle: (next: boolean) => void;
    onUninstall: () => void;
    onViewLogs: () => void;
}) {
    return (
        <li
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: 8,
                padding: space[3],
                background: palette.surface,
                display: "flex",
                alignItems: "center",
                gap: space[3],
            }}
        >
            <div style={{ flex: 1, minWidth: 0 }}>
                <div
                    style={{
                        display: "flex",
                        alignItems: "baseline",
                        gap: space[2],
                    }}
                >
                    <span
                        style={{
                            fontSize: 14,
                            fontWeight: 600,
                            color: palette.textPrimary,
                        }}
                    >
                        {plugin.name}
                    </span>
                    <span style={{ fontSize: 11, color: palette.textMuted }}>
                        v{plugin.version}
                    </span>
                </div>
                <div
                    style={{
                        fontSize: 11,
                        color: palette.textMuted,
                        fontFamily: "monospace",
                    }}
                >
                    {plugin.id}
                </div>
                {plugin.granted_capabilities.length > 0 && (
                    <div
                        style={{
                            display: "flex",
                            flexWrap: "wrap",
                            gap: space[1],
                            marginTop: space[1],
                        }}
                    >
                        {plugin.granted_capabilities.map((c) => (
                            <span
                                key={c}
                                style={{
                                    fontSize: 10,
                                    background: palette.surfaceHover,
                                    color: palette.textPrimary,
                                    padding: "2px 6px",
                                    borderRadius: 4,
                                    fontFamily: "monospace",
                                }}
                            >
                                {c}
                            </span>
                        ))}
                    </div>
                )}
            </div>

            <div style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                <Label
                    htmlFor={`enable-${plugin.id}`}
                    className="text-xs text-muted-foreground"
                >
                    Enabled
                </Label>
                <Switch
                    id={`enable-${plugin.id}`}
                    aria-label="Enabled"
                    checked={plugin.enabled}
                    onCheckedChange={(next) => onToggle(next)}
                />
                <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                        <Button
                            variant="ghost"
                            size="icon"
                            aria-label="More actions"
                        >
                            <MoreHorizontal className="size-4" />
                        </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={onViewLogs}>
                            <ScrollText className="size-4" />
                            View logs
                        </DropdownMenuItem>
                        <DropdownMenuItem
                            onClick={onUninstall}
                            variant="destructive"
                        >
                            <Trash2 className="size-4" />
                            Uninstall
                        </DropdownMenuItem>
                    </DropdownMenuContent>
                </DropdownMenu>
            </div>
        </li>
    );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
    return (
        <h3
            style={{
                fontSize: 11,
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: 0.6,
                color: palette.textMuted,
                margin: 0,
                marginBottom: space[2],
            }}
        >
            {children}
        </h3>
    );
}

function AvailablePluginRow({
    plugin,
    installing,
    onInstall,
}: {
    plugin: SystemPlugin;
    installing: boolean;
    onInstall: () => void;
}) {
    return (
        <li
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: 8,
                padding: space[3],
                background: palette.surface,
                display: "flex",
                alignItems: "center",
                gap: space[3],
            }}
        >
            <div style={{ flex: 1, minWidth: 0 }}>
                <div
                    style={{
                        display: "flex",
                        alignItems: "baseline",
                        gap: space[2],
                    }}
                >
                    <span
                        style={{
                            fontSize: 14,
                            fontWeight: 600,
                            color: palette.textPrimary,
                        }}
                    >
                        {plugin.name}
                    </span>
                    <span style={{ fontSize: 11, color: palette.textMuted }}>
                        v{plugin.version}
                    </span>
                </div>
                <div
                    style={{
                        fontSize: 11,
                        color: palette.textMuted,
                        fontFamily: "monospace",
                    }}
                >
                    {plugin.id}
                </div>
                {plugin.description && (
                    <div
                        style={{
                            fontSize: 12,
                            color: palette.textMuted,
                            marginTop: space[1],
                            lineHeight: 1.4,
                        }}
                    >
                        {plugin.description}
                    </div>
                )}
                {plugin.capabilities.length > 0 && (
                    <div
                        style={{
                            display: "flex",
                            flexWrap: "wrap",
                            gap: space[1],
                            marginTop: space[1],
                        }}
                    >
                        {plugin.capabilities.map((c) => (
                            <span
                                key={c}
                                style={{
                                    fontSize: 10,
                                    background: palette.surfaceHover,
                                    color: palette.textPrimary,
                                    padding: "2px 6px",
                                    borderRadius: 4,
                                    fontFamily: "monospace",
                                }}
                            >
                                {c}
                            </span>
                        ))}
                    </div>
                )}
            </div>

            <Button
                size="sm"
                variant="outline"
                onClick={onInstall}
                disabled={installing}
            >
                {installing ? (
                    <>
                        <Loader2 className="size-3.5 animate-spin" />
                        Installing
                    </>
                ) : (
                    <>
                        <Download className="size-3.5" />
                        Install
                    </>
                )}
            </Button>
        </li>
    );
}
