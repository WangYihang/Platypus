import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, MoreHorizontal, Trash2, ScrollText, Plus } from "lucide-react";
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
    listPlugins,
    uninstallPlugin,
} from "../../lib/api/agents/plugins";

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
export default function PluginsTab({ projectID, hostID, agentID, active }: Props) {
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

    const list = plugins.data ?? [];

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
                            onToggle={(next) => enable.mutate({ id: p.id, enabled: next })}
                            onUninstall={() => setConfirming(p)}
                            onViewLogs={() => setLogsTarget(p)}
                        />
                    ))}
                </ul>
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
