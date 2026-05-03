import { useMemo } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink, Loader2, ShieldAlert, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

import EmptyState from "../../components/EmptyState";
import { palette, space } from "../../layout/theme";
import {
    capabilityMeta,
    sortCapabilities,
} from "../../lib/capabilities";
import {
    MarketplacePlugin,
    pluginVersions,
} from "../../lib/api/marketplace";
import { humanizeError } from "../../lib/humanizeError";

// PluginDetailPage is the destination at /marketplace/plugins/:id —
// where an operator clicking a card on the marketplace listing lands
// to read the full plugin shape before authorising an install.
//
// The page shows:
//   · header (name, version, author, license, homepage)
//   · description
//   · capability list with risk metadata + human-readable labels
//   · version history (newest-first)
//
// Install-from-here is intentionally NOT in this page yet — that
// flow's natural home is the per-host PluginsTab (operator first
// picks the agent, *then* the plugin), and the dialog there does
// the cap confirmation already. Adding a global agent picker on
// this page would duplicate that surface area.
export default function PluginDetailPage() {
    const { pluginID } = useParams();
    const id = pluginID ?? "";

    const versions = useQuery({
        queryKey: ["marketplace", "versions", id],
        queryFn: () => pluginVersions(id),
        enabled: id !== "",
        refetchOnWindowFocus: false,
    });

    const latest = useMemo<MarketplacePlugin | undefined>(() => {
        const data = versions.data;
        if (!data || data.length === 0) return undefined;
        // Versions are returned newest-first by the catalog query.
        // Cross-check against latest_version when available — gives
        // us the actual latest even if the sort changes upstream.
        return (
            data.find((v) => v.version === v.latest_version) ?? data[0]
        );
    }, [versions.data]);

    if (versions.isLoading) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: space[6] }}>
                <Loader2 className="size-5 animate-spin" />
            </div>
        );
    }

    if (versions.error) {
        return (
            <div style={{ padding: space[4] }}>
                <BackLink />
                <EmptyState
                    title="Couldn't load plugin"
                    description={humanizeError(versions.error)}
                />
            </div>
        );
    }

    if (!latest) {
        return (
            <div style={{ padding: space[4] }}>
                <BackLink />
                <EmptyState
                    title="Plugin not found"
                    description={`No marketplace entry for ${id}. The catalog may be stale — try refreshing from /marketplace.`}
                />
            </div>
        );
    }

    return (
        <div style={{ padding: space[4], display: "flex", flexDirection: "column", gap: space[4] }}>
            <BackLink />
            <Header plugin={latest} />
            {latest.description && (
                <p style={{ margin: 0, fontSize: 13, color: palette.textSecondary }}>
                    {latest.description}
                </p>
            )}
            <CapabilitiesSection plugin={latest} />
            <VersionsTable
                versions={versions.data ?? []}
                latestVersion={latest.latest_version}
            />
        </div>
    );
}

function BackLink() {
    return (
        <Link
            to="/marketplace"
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: space[1],
                fontSize: 12,
                color: palette.textMuted,
                textDecoration: "none",
            }}
        >
            <ArrowLeft className="size-3.5" />
            Back to Marketplace
        </Link>
    );
}

function Header({ plugin }: { plugin: MarketplacePlugin }) {
    return (
        <header style={{ display: "flex", flexDirection: "column", gap: space[1] }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: space[2] }}>
                <h1
                    style={{
                        margin: 0,
                        fontSize: 22,
                        fontWeight: 600,
                        color: palette.textPrimary,
                    }}
                >
                    {plugin.name}
                </h1>
                <span style={{ fontSize: 13, color: palette.textMuted }}>
                    v{plugin.version}
                </span>
            </div>
            <div
                style={{
                    fontSize: 12,
                    color: palette.textMuted,
                    fontFamily: "monospace",
                }}
            >
                {plugin.plugin_id}
            </div>
            <div
                style={{
                    display: "flex",
                    flexWrap: "wrap",
                    gap: space[3],
                    fontSize: 12,
                    color: palette.textMuted,
                    marginTop: space[1],
                }}
            >
                {plugin.author && <span>by {plugin.author}</span>}
                {plugin.license && <span>{plugin.license}</span>}
                {plugin.homepage && (
                    <a
                        href={plugin.homepage}
                        target="_blank"
                        rel="noreferrer"
                        style={{
                            color: palette.accent,
                            textDecoration: "none",
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 2,
                        }}
                    >
                        Homepage
                        <ExternalLink className="size-3" />
                    </a>
                )}
            </div>
        </header>
    );
}

function CapabilitiesSection({ plugin }: { plugin: MarketplacePlugin }) {
    if (plugin.capabilities.length === 0) {
        return (
            <section>
                <h2 style={sectionHeader}>Capabilities</h2>
                <p style={{ margin: 0, fontSize: 12, color: palette.textMuted }}>
                    Declares no host capabilities. Sandboxed but can't read
                    files, execute commands, or talk to the network.
                </p>
            </section>
        );
    }
    const sorted = sortCapabilities(plugin.capabilities.map((f) => ({ family: f })));
    return (
        <section>
            <h2 style={sectionHeader}>Capabilities</h2>
            <ul
                style={{
                    listStyle: "none",
                    padding: 0,
                    margin: 0,
                    display: "flex",
                    flexDirection: "column",
                    gap: space[1],
                }}
            >
                {sorted.map(({ family }) => {
                    const meta = capabilityMeta(family);
                    return (
                        <li
                            key={family}
                            style={{
                                display: "flex",
                                alignItems: "flex-start",
                                gap: space[2],
                                padding: space[2],
                                border: `1px solid ${palette.border}`,
                                borderRadius: 6,
                                background: palette.surface,
                            }}
                        >
                            <RiskIcon risk={meta.risk} />
                            <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                                <div
                                    style={{
                                        display: "flex",
                                        gap: space[2],
                                        alignItems: "center",
                                    }}
                                >
                                    <span style={{ fontSize: 13, fontWeight: 600 }}>
                                        {meta.label}
                                    </span>
                                    <RiskBadge risk={meta.risk} />
                                    <span
                                        style={{
                                            fontSize: 11,
                                            fontFamily: "monospace",
                                            color: palette.textMuted,
                                        }}
                                    >
                                        {family}
                                    </span>
                                </div>
                                <span style={{ fontSize: 12, color: palette.textSecondary }}>
                                    {meta.summary}
                                </span>
                            </div>
                        </li>
                    );
                })}
            </ul>
        </section>
    );
}

function VersionsTable({
    versions,
    latestVersion,
}: {
    versions: MarketplacePlugin[];
    latestVersion: string;
}) {
    return (
        <section>
            <h2 style={sectionHeader}>Versions</h2>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
                <thead>
                    <tr style={{ textAlign: "left", color: palette.textMuted }}>
                        <th style={{ padding: space[2] }}>Version</th>
                        <th style={{ padding: space[2] }}>Cached</th>
                        <th style={{ padding: space[2] }}>Capabilities</th>
                    </tr>
                </thead>
                <tbody>
                    {versions.map((v) => (
                        <tr
                            key={v.version}
                            style={{ borderTop: `1px solid ${palette.border}` }}
                        >
                            <td style={{ padding: space[2], fontFamily: "monospace" }}>
                                {v.version}
                                {v.version === latestVersion && (
                                    <Badge variant="secondary" className="ml-2">
                                        latest
                                    </Badge>
                                )}
                            </td>
                            <td style={{ padding: space[2], color: palette.textMuted }}>
                                {new Date(v.fetched_at_unix * 1000).toLocaleString()}
                            </td>
                            <td style={{ padding: space[2], color: palette.textMuted }}>
                                {v.capabilities.length === 0
                                    ? "—"
                                    : v.capabilities.join(", ")}
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
            {versions.length === 0 && (
                <div style={{ fontSize: 12, color: palette.textMuted, padding: space[2] }}>
                    No versions cached.
                </div>
            )}
        </section>
    );
}

function RiskIcon({ risk }: { risk: "low" | "medium" | "high" }) {
    if (risk === "high") return <ShieldAlert className="size-4 text-red-600 mt-0.5" />;
    if (risk === "medium") return <ShieldAlert className="size-4 text-amber-600 mt-0.5" />;
    return <ShieldCheck className="size-4 text-emerald-600 mt-0.5" />;
}

function RiskBadge({ risk }: { risk: "low" | "medium" | "high" }) {
    if (risk === "high") return <Badge variant="destructive">High risk</Badge>;
    if (risk === "medium") return <Badge variant="secondary">Medium risk</Badge>;
    return <Badge variant="outline">Low risk</Badge>;
}

const sectionHeader: React.CSSProperties = {
    margin: 0,
    fontSize: 14,
    fontWeight: 600,
    color: palette.textPrimary,
    marginBottom: space[2],
};

// Surface a button-typed back-affordance too — the lightweight Link
// is enough but Button helps the design system regression suite catch
// future rev styling drift. Currently unused; reserved.
const _BackButton = () => (
    <Button asChild variant="ghost" size="sm">
        <Link to="/marketplace">Back</Link>
    </Button>
);
