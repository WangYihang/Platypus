import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, Monitor, RotateCw, Search } from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusDot from "../components/StatusDot";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { Host, listHosts } from "../lib/api";
import { fromNow, isOnline } from "../lib/time";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

// HostsPage is the cross-host list for a project. Solves the IA gap
// where hosts were only reachable via sidebar tree expansion. Always
// landable, always shows the empty-state path to creating a listener
// (which is how new hosts arrive).
export default function HostsPage() {
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
            toast.error(`load hosts: ${String(e)}`);
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
            [h.hostname, h.primary_alias, h.os, h.machine_id]
                .filter(Boolean)
                .some((v) => String(v).toLowerCase().includes(q)),
        );
    }, [hosts, query]);

    const onlineCount = hosts?.filter((h) => isOnline(h.last_seen_at)).length ?? 0;

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Hosts"
                subtitle={
                    hosts === null
                        ? "Loading…"
                        : `${hosts.length} total · ${onlineCount} online`
                }
                actions={
                    <Button size="sm" variant="outline" disabled={loading} onClick={refresh}>
                        {loading ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <RotateCw className="size-3.5" />
                        )}
                        Refresh
                    </Button>
                }
            />
            <Toolbar
                left={
                    <div className="relative max-w-[360px] w-full">
                        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-text-muted pointer-events-none" />
                        <Input
                            placeholder="Search hostname, alias, OS"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            className="h-8 pl-8"
                        />
                    </div>
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
                        description="Hosts register themselves when an agent connects to one of your listeners. Create a listener first, then run the agent on a target machine."
                        action={
                            <Button
                                onClick={() => navigate(`/projects/${project.slug}/listeners`)}
                            >
                                Manage listeners
                            </Button>
                        }
                    />
                ) : !hosts ? (
                    <div className="flex items-center justify-center p-20">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : filtered && filtered.length === 0 ? (
                    <EmptyState title="No matches" description={`No host matches "${query}".`} />
                ) : (
                    <Card padding={0}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Host</TableHead>
                                    <TableHead className="w-[180px]">OS · platform</TableHead>
                                    <TableHead className="w-[100px]">Arch</TableHead>
                                    <TableHead className="w-[140px]">Primary IP</TableHead>
                                    <TableHead className="w-[90px]">CPU</TableHead>
                                    <TableHead className="w-[110px]">Memory</TableHead>
                                    <TableHead className="w-[160px]">Machine ID</TableHead>
                                    <TableHead className="w-[140px]">Last seen</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {(filtered ?? []).map((h) => {
                                    const primary =
                                        h.primary_alias ||
                                        h.hostname ||
                                        h.machine_id?.slice(0, 8) ||
                                        "unknown";
                                    return (
                                        <TableRow
                                            key={h.id}
                                            className="cursor-pointer"
                                            onClick={() =>
                                                navigate(
                                                    `/projects/${project.slug}/hosts/${h.id}/terminal`,
                                                )
                                            }
                                        >
                                            <TableCell>
                                                <div className="flex items-center gap-2">
                                                    <StatusDot
                                                        status={
                                                            isOnline(h.last_seen_at)
                                                                ? "online"
                                                                : "offline"
                                                        }
                                                    />
                                                    <span className="font-medium text-text-primary">
                                                        {primary}
                                                    </span>
                                                </div>
                                            </TableCell>
                                            <TableCell>
                                                {renderOSCell(h)}
                                            </TableCell>
                                            <TableCell>
                                                {h.arch ? (
                                                    <Mono>{h.arch}</Mono>
                                                ) : (
                                                    <span className="text-text-muted">—</span>
                                                )}
                                            </TableCell>
                                            <TableCell>
                                                {h.primary_ip ? (
                                                    <Mono>{h.primary_ip}</Mono>
                                                ) : (
                                                    <span className="text-text-muted">—</span>
                                                )}
                                            </TableCell>
                                            <TableCell className="text-text-secondary">
                                                {h.num_cpu ? `${h.num_cpu}×` : "—"}
                                            </TableCell>
                                            <TableCell className="text-text-secondary">
                                                {formatMem(h.mem_total_bytes)}
                                            </TableCell>
                                            <TableCell>
                                                {h.machine_id ? (
                                                    <Mono>{`${h.machine_id.slice(0, 12)}…`}</Mono>
                                                ) : (
                                                    <span className="text-text-muted">fp</span>
                                                )}
                                            </TableCell>
                                            <TableCell className="text-text-secondary">
                                                {fromNow(h.last_seen_at)}
                                            </TableCell>
                                        </TableRow>
                                    );
                                })}
                            </TableBody>
                        </Table>
                    </Card>
                )}
            </div>
        </div>
    );
}

// renderOSCell picks the richest label we have — "ubuntu 22.04" beats
// just "linux" — and falls back to "—" when the agent never reported
// anything. Called inline in the table body to keep the row JSX tidy.
function renderOSCell(h: Host) {
    const parts: string[] = [];
    if (h.platform) {
        parts.push(h.platform + (h.platform_version ? ` ${h.platform_version}` : ""));
    } else if (h.os) {
        parts.push(h.os);
    }
    if (parts.length === 0) {
        return <span className="text-text-muted">—</span>;
    }
    return <span>{parts.join(" · ")}</span>;
}

// formatMem collapses a byte count into a compact GB/TB label for
// the hosts list column. Hosts with unknown memory show "—".
function formatMem(n?: number): string {
    if (!n || n <= 0) return "—";
    const gb = n / (1024 * 1024 * 1024);
    if (gb < 1) return `${Math.round(n / (1024 * 1024))} MB`;
    if (gb >= 1024) return `${(gb / 1024).toFixed(1)} TB`;
    return `${gb.toFixed(gb >= 10 ? 0 : 1)} GB`;
}
