import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import EmptyState from "../components/EmptyState";
import { palette, space } from "../layout/theme";
import {
    MarketplacePlugin,
    MarketplaceRefreshStatus,
    refreshCatalog,
    refreshStatus,
    searchPlugins,
} from "../lib/api/marketplace";
import { humanizeError } from "../lib/humanizeError";
import { icons } from "../lib/icons";

// /marketplace — global page (not project-scoped) listing plugins
// from the server's cached marketplace catalog. The cache is a SQLite
// mirror of the platypus-plugins index repo (see
// internal/core/plugin/catalog.go); a refresh hits the index URL
// configured via PLATYPUS_PLUGIN_INDEX server env.
//
// Per-agent install + capability-confirm dialog deliberately not in
// this page yet — operator picks a plugin from here, then navigates
// to the per-host page (forthcoming) to install on a target.
export default function MarketplacePage() {
    const [query, setQuery] = useState("");
    const queryClient = useQueryClient();

    const plugins = useQuery({
        queryKey: ["marketplace", "search", query],
        queryFn: () => searchPlugins(query),
    });

    const status = useQuery<MarketplaceRefreshStatus | "never_synced">({
        queryKey: ["marketplace", "status"],
        queryFn: refreshStatus,
        // Cheap; refresh status changes only on operator-triggered
        // refresh or the periodic worker. 30s background refetch is
        // plenty.
        refetchInterval: 30_000,
    });

    const refresh = useMutation({
        mutationFn: refreshCatalog,
        onSuccess: (count) => {
            toast.success(`Marketplace synced: ${count} plugin versions`);
            void queryClient.invalidateQueries({ queryKey: ["marketplace"] });
        },
        onError: (err) => toast.error(humanizeError(err)),
    });

    const I = icons;

    return (
        <div style={{ padding: space[4], display: "flex", flexDirection: "column", gap: space[3] }}>
            <header
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                }}
            >
                <div>
                    <h1
                        style={{
                            display: "flex",
                            alignItems: "center",
                            gap: space[2],
                            margin: 0,
                            fontSize: 18,
                            fontWeight: 600,
                            color: palette.textPrimary,
                        }}
                    >
                        <I.marketplace className="size-5" />
                        Marketplace
                    </h1>
                    <p style={{ margin: 0, color: palette.textMuted, fontSize: 12 }}>
                        Community plugins cached from the platypus-plugins index. Pick one to
                        install on a host.
                    </p>
                </div>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={() => refresh.mutate()}
                    disabled={refresh.isPending}
                >
                    {refresh.isPending ? (
                        <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                        <RefreshCw className="size-3.5" />
                    )}
                    Refresh
                </Button>
            </header>

            <RefreshStatusLine status={status.data} />

            <Input
                placeholder="Search by plugin name…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
            />

            {plugins.isLoading ? (
                <div
                    style={{
                        display: "flex",
                        justifyContent: "center",
                        padding: space[6],
                    }}
                >
                    <Loader2 className="size-5 animate-spin" />
                </div>
            ) : plugins.data && plugins.data.length > 0 ? (
                <PluginList plugins={plugins.data} />
            ) : (
                <EmptyState
                    title="No plugins found"
                    description={
                        query
                            ? `No marketplace plugin matches "${query}".`
                            : "The marketplace catalog is empty. Configure PLATYPUS_PLUGIN_INDEX on the server and click Refresh."
                    }
                />
            )}
        </div>
    );
}

function RefreshStatusLine({
    status,
}: {
    status?: MarketplaceRefreshStatus | "never_synced";
}) {
    if (!status || status === "never_synced") {
        return (
            <div style={{ fontSize: 12, color: palette.textMuted }}>
                Catalog has never been synced.
            </div>
        );
    }
    const when = new Date(status.last_fetched_unix * 1000).toLocaleString();
    const ok = status.last_status === "ok";
    return (
        <div
            style={{
                fontSize: 12,
                color: ok ? palette.textMuted : palette.danger,
                display: "flex",
                gap: space[1],
            }}
        >
            <span>Last sync: {when}</span>
            <span>•</span>
            <span>{status.plugin_count} plugin versions</span>
            {!ok && (
                <>
                    <span>•</span>
                    <span>error: {status.last_error || status.last_status}</span>
                </>
            )}
        </div>
    );
}

function PluginList({ plugins }: { plugins: MarketplacePlugin[] }) {
    return (
        <ul
            style={{
                listStyle: "none",
                padding: 0,
                margin: 0,
                display: "grid",
                gap: space[2],
                gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))",
            }}
        >
            {plugins.map((p) => (
                <PluginCard key={`${p.plugin_id}@${p.version}`} plugin={p} />
            ))}
        </ul>
    );
}

function PluginCard({ plugin }: { plugin: MarketplacePlugin }) {
    return (
        <li
            style={{
                border: `1px solid ${palette.border}`,
                borderRadius: 8,
                padding: space[3],
                display: "flex",
                flexDirection: "column",
                gap: space[1],
                background: palette.surface,
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "baseline",
                    justifyContent: "space-between",
                }}
            >
                <h3
                    style={{
                        margin: 0,
                        fontSize: 14,
                        fontWeight: 600,
                        color: palette.textPrimary,
                    }}
                >
                    {plugin.name}
                </h3>
                <span style={{ fontSize: 11, color: palette.textMuted }}>v{plugin.version}</span>
            </div>
            <div
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    fontFamily: "monospace",
                }}
            >
                {plugin.plugin_id}
            </div>
            {plugin.description && (
                <p style={{ margin: 0, fontSize: 12, color: palette.textSecondary }}>
                    {plugin.description}
                </p>
            )}
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
            <div
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    display: "flex",
                    justifyContent: "space-between",
                    marginTop: space[1],
                }}
            >
                <span>{plugin.author || "unknown"}</span>
                {plugin.license && <span>{plugin.license}</span>}
            </div>
        </li>
    );
}
