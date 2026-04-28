import { useCallback, useEffect, useMemo, useState } from "react";
import {
    Boxes,
    Cpu,
    HelpCircle,
    Laptop,
    Layers,
    Loader2,
    Monitor,
    Search,
    Server,
} from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";

import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import StatusDot from "../../components/StatusDot";
import Toolbar from "../../components/Toolbar";
import { useCurrentProject } from "../../layout/ProjectShell";
import { palette, radius, space } from "../../layout/theme";
import { Host, listHosts } from "../../lib/api";
import { humanizeError } from "../../lib/humanizeError";
import { fromNow, isOnline } from "../../lib/time";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";

// HostsCardPanel renders the same fleet inventory as HostsPanel but as
// a responsive grid of cards instead of a table. Each card highlights
// the host's identity (alias / hostname), live status, and the headline
// hardware facts (OS, arch, CPU, memory) at a glance — better for
// smaller fleets where scanning rows feels heavyweight.
export default function HostsCardPanel() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const [hosts, setHosts] = useState<Host[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [query, setQuery] = useState("");

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setHosts(await listHosts(project.id));
            setError(null);
        } catch (e) {
            setError(String(e));
            toast.error(`load hosts: ${humanizeError(e)}`);
        } finally {
            setLoading(false);
        }
    }, [project.id]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const filtered = useMemo(() => {
        if (!hosts) return null;
        const q = query.trim().toLowerCase();
        if (!q) return hosts;
        return hosts.filter((h) =>
            [h.hostname, h.primary_alias, h.os, h.machine_id, h.primary_ip]
                .filter(Boolean)
                .some((v) => String(v).toLowerCase().includes(q)),
        );
    }, [hosts, query]);

    const onlineCount = hosts?.filter((h) => isOnline(h.last_seen_at)).length ?? 0;

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <Toolbar
                left={
                    <div className="relative max-w-[360px] w-full">
                        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-text-muted pointer-events-none" />
                        <Input
                            placeholder="Search hostname, alias, OS, IP"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            className="h-8 pl-8"
                        />
                    </div>
                }
                right={
                    <span style={{ color: palette.textMuted, fontSize: 12 }}>
                        {hosts === null
                            ? "Loading…"
                            : `${hosts.length} total · ${onlineCount} online`}
                        {loading && hosts !== null && (
                            <Loader2
                                className="size-3.5 animate-spin inline-block ml-2"
                                style={{ verticalAlign: "middle" }}
                            />
                        )}
                    </span>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <div
                        style={{
                            marginBottom: space[4],
                            padding: `${space[3]}px ${space[4]}px`,
                            border: `1px solid ${palette.danger}`,
                            borderRadius: 6,
                            color: palette.danger,
                            fontSize: 13,
                        }}
                    >
                        {error}
                    </div>
                )}
                {hosts && hosts.length === 0 ? (
                    <EmptyState
                        icon={<Monitor className="size-5" />}
                        title="No hosts yet"
                        description="Hosts appear here once an agent enrolls into this project. Generate an install command or enrollment token, then run the agent on the target machine."
                        action={
                            <Button
                                onClick={() => navigate(`/projects/${project.slug}/fleet/enroll`)}
                            >
                                Enroll agent
                            </Button>
                        }
                    />
                ) : !hosts ? (
                    <CardGridSkeleton />
                ) : filtered && filtered.length === 0 ? (
                    <EmptyState title="No matches" description={`No host matches "${query}".`} />
                ) : (
                    <div
                        data-testid="fleet-cards-grid"
                        style={{
                            display: "grid",
                            gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
                            gap: space[3],
                        }}
                    >
                        {(filtered ?? []).map((h) => (
                            <HostCard
                                key={h.id}
                                host={h}
                                onOpen={() =>
                                    navigate(`/projects/${project.slug}/hosts/${h.id}/info`)
                                }
                            />
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}

interface HostCardProps {
    host: Host;
    onOpen: () => void;
}

function HostCard({ host, onOpen }: HostCardProps) {
    const online = isOnline(host.last_seen_at);
    const primary =
        host.primary_alias ||
        host.hostname ||
        host.machine_id?.slice(0, 8) ||
        "unknown";

    return (
        <button
            type="button"
            onClick={onOpen}
            data-testid="fleet-card"
            data-online={online ? "true" : "false"}
            style={{
                textAlign: "left",
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                padding: `${space[4]}px ${space[4]}px ${space[3]}px`,
                cursor: "pointer",
                display: "flex",
                flexDirection: "column",
                gap: space[3],
                transition: "border-color 120ms ease, background 120ms ease",
                color: palette.textPrimary,
                fontFamily: "var(--font-geist-sans)",
            }}
            onMouseEnter={(e) =>
                (e.currentTarget.style.borderColor = palette.borderStrong)
            }
            onMouseLeave={(e) =>
                (e.currentTarget.style.borderColor = palette.border)
            }
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    minWidth: 0,
                }}
            >
                <span style={{ flexShrink: 0 }}>
                    {renderMachineTypeIcon(host.machine_type)}
                </span>
                <StatusDot status={online ? "online" : "offline"} />
                <span
                    style={{
                        fontWeight: 600,
                        fontSize: 14,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        flex: 1,
                        minWidth: 0,
                    }}
                    title={primary}
                >
                    {primary}
                </span>
            </div>
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "auto 1fr",
                    rowGap: 4,
                    columnGap: space[2],
                    fontSize: 12,
                    color: palette.textSecondary,
                }}
            >
                <span style={{ color: palette.textMuted }}>OS</span>
                <span
                    style={{
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {renderOSLabel(host)}
                </span>
                <span style={{ color: palette.textMuted }}>Arch</span>
                <span>
                    {host.arch ? <Mono>{host.arch}</Mono> : <Dim>—</Dim>}
                </span>
                <span style={{ color: palette.textMuted }}>IP</span>
                <span>
                    {host.primary_ip ? (
                        <Mono>{host.primary_ip}</Mono>
                    ) : (
                        <Dim>—</Dim>
                    )}
                </span>
                <span style={{ color: palette.textMuted }}>Hardware</span>
                <span
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: space[2],
                    }}
                >
                    <span
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 4,
                        }}
                    >
                        <Cpu className="size-3" />
                        {host.num_cpu ? `${host.num_cpu}×` : "—"}
                    </span>
                    <span style={{ color: palette.border }}>·</span>
                    <span>{formatMem(host.mem_total_bytes)}</span>
                </span>
            </div>
            <div
                style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    fontSize: 11,
                    color: palette.textMuted,
                    borderTop: `1px solid ${palette.border}`,
                    paddingTop: space[2],
                }}
            >
                <span>
                    {host.machine_id ? (
                        <Mono size={11}>{host.machine_id.slice(0, 12)}…</Mono>
                    ) : (
                        "fp pending"
                    )}
                </span>
                <span>{fromNow(host.last_seen_at)}</span>
            </div>
        </button>
    );
}

function Dim({ children }: { children: React.ReactNode }) {
    return <span style={{ color: palette.textMuted }}>{children}</span>;
}

function renderOSLabel(h: Host): React.ReactNode {
    if (h.platform) {
        const v = h.platform_version ? ` ${h.platform_version}` : "";
        return `${h.platform}${v}`;
    }
    if (h.os) return h.os;
    return <Dim>—</Dim>;
}

function formatMem(n?: number): string {
    if (!n || n <= 0) return "—";
    const gb = n / (1024 * 1024 * 1024);
    if (gb < 1) return `${Math.round(n / (1024 * 1024))} MB`;
    if (gb >= 1024) return `${(gb / 1024).toFixed(1)} TB`;
    return `${gb.toFixed(gb >= 10 ? 0 : 1)} GB`;
}

const machineTypeIcons: Record<
    string,
    { label: string; Icon: React.ComponentType<{ className?: string }> }
> = {
    container: { label: "container", Icon: Boxes },
    vm: { label: "virtual machine", Icon: Layers },
    bare_metal: { label: "bare metal", Icon: Server },
    laptop: { label: "laptop", Icon: Laptop },
    desktop: { label: "desktop", Icon: Monitor },
    unknown: { label: "unknown", Icon: HelpCircle },
};

function renderMachineTypeIcon(type?: string) {
    const meta = type ? machineTypeIcons[type] : undefined;
    if (!meta) return <HelpCircle className="size-4 text-text-muted" />;
    const { Icon } = meta;
    return <Icon className="size-4 text-text-secondary" />;
}

function CardGridSkeleton() {
    const rows = Array.from({ length: 6 });
    return (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
                gap: space[3],
            }}
        >
            {rows.map((_, i) => (
                <div
                    key={i}
                    data-testid="hosts-card-skeleton"
                    style={{
                        background: palette.surface,
                        border: `1px solid ${palette.border}`,
                        borderRadius: radius.md,
                        padding: space[4],
                        display: "flex",
                        flexDirection: "column",
                        gap: space[3],
                    }}
                >
                    <Skeleton className="h-4 w-3/4" />
                    <Skeleton className="h-3 w-1/2" />
                    <Skeleton className="h-3 w-2/3" />
                    <Skeleton className="h-3 w-1/3" />
                </div>
            ))}
        </div>
    );
}
