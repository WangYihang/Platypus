import { ReactNode, useState } from "react";
import { CheckCircle2, Loader2, Plus, ShieldAlert } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";

import EmptyState from "../../components/EmptyState";
import { palette, radius, space } from "../../layout/theme";
import { capabilityMeta } from "../../lib/capabilities";
import { humanizeError } from "../../lib/humanizeError";
import {
    InstalledPlugin,
    installFromSystem,
} from "../../lib/api/agents/plugins";
import { listSystemPlugins, SystemPlugin } from "../../lib/api/system_plugins";
import { useQuery } from "@tanstack/react-query";

import { Activity } from "./ActivityBar";
import { missingFor, useInstalledPluginIDs } from "../../lib/activityPlugins";
import { useHostContext } from "./HostContext";

interface Props {
    projectID: string;
    agentID: string;
    activity: Activity;
    children: ReactNode;
}

// RequiresPlugins is the activity-tab guard that swaps the body
// for an install guide when the operator's allowlist didn't
// include all plugins the tab needs.
//
// Lifecycle:
//   1. Pulls installed plugins via useInstalledPluginIDs (shared
//      query-key with PluginsTab so a successful install there
//      reactively re-renders this tab).
//   2. Computes missing = required − installed.
//   3. While the query is loading or all plugins are present,
//      passes children through unchanged — the tab's own loader
//      paints during that brief window.
//   4. Otherwise renders InstallGuide which lists the missing
//      plugins, surfaces their declared capabilities (so the
//      operator sees what they're authorising), and provides a
//      one-click "Install all" button that installs each missing
//      plugin from the system catalog with the same capabilities
//      it already declares (system plugins are pre-vetted by the
//      publisher key, so we don't re-prompt for per-cap approval
//      — that gate already happened at enroll time when the
//      operator picked the baseline allowlist).
export default function RequiresPlugins({
    projectID,
    agentID,
    activity,
    children,
}: Props) {
    const installed = useInstalledPluginIDs(projectID, agentID);
    const { host } = useHostContext();
    const missing = missingFor(activity, installed.ids, host.os ?? "");

    if (installed.isError) {
        return (
            <EmptyState
                title="Couldn't check installed plugins"
                description="The agent didn't respond. The tab can't gate on plugin availability without that data."
            />
        );
    }
    if (missing.length === 0) {
        return <>{children}</>;
    }
    return (
        <InstallGuide
            projectID={projectID}
            agentID={agentID}
            activity={activity}
            missingIDs={missing}
        />
    );
}

// InstallGuide is the panel rendered in place of the tab body when
// one or more required plugins are missing. It mirrors the system
// catalog (so the operator sees the plugin name + capabilities it
// would grant, not just an opaque id) and lets them install each
// missing plugin individually OR all at once.
function InstallGuide({
    projectID,
    agentID,
    activity,
    missingIDs,
}: {
    projectID: string;
    agentID: string;
    activity: Activity;
    missingIDs: string[];
}) {
    const queryClient = useQueryClient();
    const catalog = useQuery({
        queryKey: ["system-plugins-catalog"],
        queryFn: () => listSystemPlugins(),
        staleTime: 5 * 60 * 1000, // 5min — catalog doesn't churn
    });
    const [installing, setInstalling] = useState<Set<string>>(new Set());

    const installOne = useMutation({
        mutationFn: async (entry: SystemPlugin) => {
            // System plugins live on the server's local disk under
            // <data-dir>/system-plugins/. The dedicated install_system
            // endpoint reads them from there + streams to the agent
            // via the same install pipeline the marketplace path uses.
            // The capability set we pass in is what the manifest
            // declares — system plugins are signed by the trusted
            // publisher key, so granting their declared caps wholesale
            // matches the trust the operator already gave at enroll
            // time when they accepted the system bundle.
            return installFromSystem(projectID, agentID, {
                pluginID: entry.id,
                version: entry.version,
                grantedCapabilities: entry.capabilities,
            });
        },
        onMutate: (entry) => {
            setInstalling((s) => new Set(s).add(entry.id));
        },
        onSettled: (_data, _err, entry) => {
            setInstalling((s) => {
                const next = new Set(s);
                next.delete(entry.id);
                return next;
            });
        },
        onSuccess: (_res, entry) => {
            toast.success(`Installed ${entry.name}`);
            void queryClient.invalidateQueries({
                queryKey: ["agent-plugins", projectID, agentID],
            });
        },
        onError: (err) => {
            toast.error(humanizeError(err));
        },
    });

    if (catalog.isLoading) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: space[6] }}>
                <Loader2 className="size-5 animate-spin" />
            </div>
        );
    }
    if (catalog.error) {
        return (
            <EmptyState
                title="Couldn't load the plugin catalog"
                description={humanizeError(catalog.error)}
            />
        );
    }

    const catalogByID = new Map(
        (catalog.data ?? []).map((p) => [p.id, p] as const),
    );
    const entries: { id: string; entry: SystemPlugin | null }[] = missingIDs.map(
        (id) => ({ id, entry: catalogByID.get(id) ?? null }),
    );
    const installable = entries
        .map((e) => e.entry)
        .filter((e): e is SystemPlugin => e !== null);
    const allInstalling = installable.length > 0 && installable.every((e) => installing.has(e.id));

    return (
        <div
            data-testid={`requires-plugins-${activity}`}
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[4],
                padding: space[5],
                maxWidth: 720,
                margin: "0 auto",
            }}
        >
            <div style={{ display: "flex", gap: space[3], alignItems: "flex-start" }}>
                <ShieldAlert className="size-6 shrink-0" style={{ color: palette.warning }} />
                <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                    <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>
                        This view needs {missingIDs.length === 1 ? "1 plugin" : `${missingIDs.length} plugins`}
                    </h2>
                    <p
                        style={{
                            margin: 0,
                            fontSize: 13,
                            color: palette.textSecondary,
                        }}
                    >
                        Capabilities here run inside signed wasm plugins. The
                        agent's baseline didn't include the plugins this tab
                        depends on — install them below to enable it. You can
                        also install via the Plugins tab on the left.
                    </p>
                </div>
            </div>

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
                {entries.map(({ id, entry }) => (
                    <li
                        key={id}
                        style={{
                            display: "flex",
                            gap: space[3],
                            alignItems: "flex-start",
                            border: `1px solid ${palette.border}`,
                            borderRadius: radius.md,
                            padding: space[3],
                            background: palette.surface,
                        }}
                    >
                        <div
                            style={{
                                display: "flex",
                                flexDirection: "column",
                                gap: 2,
                                flex: 1,
                                minWidth: 0,
                            }}
                        >
                            <div
                                style={{
                                    display: "flex",
                                    alignItems: "baseline",
                                    gap: space[2],
                                }}
                            >
                                <span style={{ fontSize: 14, fontWeight: 600 }}>
                                    {entry?.name ?? id}
                                </span>
                                {entry && (
                                    <span
                                        style={{
                                            fontSize: 11,
                                            color: palette.textMuted,
                                        }}
                                    >
                                        v{entry.version}
                                    </span>
                                )}
                            </div>
                            <span
                                style={{
                                    fontSize: 11,
                                    color: palette.textMuted,
                                    fontFamily: "monospace",
                                }}
                            >
                                {id}
                            </span>
                            {entry?.description && (
                                <p
                                    style={{
                                        margin: "4px 0 0",
                                        fontSize: 12,
                                        color: palette.textSecondary,
                                    }}
                                >
                                    {entry.description}
                                </p>
                            )}
                            {entry && entry.capabilities.length > 0 && (
                                <div
                                    style={{
                                        marginTop: 6,
                                        display: "flex",
                                        flexWrap: "wrap",
                                        gap: 4,
                                    }}
                                >
                                    {entry.capabilities.map((c) => (
                                        <span
                                            key={c}
                                            title={capabilityMeta(c).summary}
                                            style={{
                                                fontSize: 10,
                                                padding: "2px 6px",
                                                borderRadius: 4,
                                                background: palette.surfaceHover,
                                                color: palette.textPrimary,
                                                fontFamily: "monospace",
                                            }}
                                        >
                                            {c}
                                        </span>
                                    ))}
                                </div>
                            )}
                            {!entry && (
                                <p
                                    style={{
                                        margin: "4px 0 0",
                                        fontSize: 12,
                                        color: palette.warning,
                                    }}
                                >
                                    Not in the server's system-plugin catalog. Ask
                                    your operator to seed{" "}
                                    <code>{"<data-dir>/system-plugins/"}</code>{" "}
                                    or pick a different baseline at enroll time.
                                </p>
                            )}
                        </div>
                        <div style={{ flexShrink: 0 }}>
                            {entry ? (
                                <Button
                                    size="sm"
                                    onClick={() => installOne.mutate(entry)}
                                    disabled={installing.has(entry.id)}
                                >
                                    {installing.has(entry.id) ? (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    ) : (
                                        <Plus className="size-3.5" />
                                    )}
                                    Install
                                </Button>
                            ) : (
                                <Button size="sm" disabled>
                                    Unavailable
                                </Button>
                            )}
                        </div>
                    </li>
                ))}
            </ul>

            {installable.length > 1 && (
                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                    <Button
                        onClick={() => {
                            for (const entry of installable) {
                                if (!installing.has(entry.id)) {
                                    installOne.mutate(entry);
                                }
                            }
                        }}
                        disabled={allInstalling}
                    >
                        {allInstalling ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <CheckCircle2 className="size-3.5" />
                        )}
                        Install all {installable.length}
                    </Button>
                </div>
            )}
        </div>
    );
}
