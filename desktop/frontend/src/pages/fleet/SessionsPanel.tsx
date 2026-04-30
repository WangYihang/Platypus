import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { humanizeError } from "../../lib/humanizeError";
import { useNavigate } from "react-router-dom";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import FilterToolbar from "../../components/FilterToolbar";
import { useCurrentProject } from "../../layout/ProjectShell";
import { palette, space } from "../../layout/theme";
import {
    Host,
    SessionRow,
    listHosts,
    listProjectSessions,
} from "../../lib/api";
import { qk } from "../../lib/queryKeys";
import { fromNow } from "../../lib/time";

import { Input } from "@/components/ui/input";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

type FilterMode = "live" | "all";

// SessionsPanel is the Fleet page's timeline view. Lives under
// FleetPage along with HostsPanel and TopologyPanel; Fleet toggles
// visibility with display:none so re-opening Timeline is instant and
// filter/search state is preserved across the switch.
export default function SessionsPanel() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const queryClient = useQueryClient();
    const [filter, setFilter] = useState<FilterMode>("live");
    const [query, setQuery] = useState("");

    const sessionsKey = ["projectSessions", project.id, { live: filter === "live" }] as const;
    const sessionsQuery = useQuery({
        queryKey: sessionsKey,
        queryFn: () =>
            listProjectSessions(project.id, filter === "live" ? { live: true } : {}),
    });
    const hostsQuery = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });

    const sessions: SessionRow[] | null = sessionsQuery.data ?? null;
    const hostsByID = useMemo(() => {
        const m: Record<string, Host> = {};
        for (const x of hostsQuery.data ?? []) m[x.id] = x;
        return m;
    }, [hostsQuery.data]);
    const error = sessionsQuery.error ?? hostsQuery.error ?? null;
    const loading = sessionsQuery.isFetching || hostsQuery.isFetching;
    const refresh = () => {
        queryClient.invalidateQueries({ queryKey: sessionsKey });
        queryClient.invalidateQueries({ queryKey: qk.hosts(project.id) });
    };

    useEffect(() => {
        if (error) toast.error(`load sessions: ${humanizeError(error)}`);
    }, [error]);

    const filtered = useMemo(() => {
        if (!sessions) return null;
        const q = query.trim().toLowerCase();
        if (!q) return sessions;
        return sessions.filter((s) => {
            const host = hostsByID[s.host_id];
            const hay = [
                s.id,
                s.user,
                s.remote_addr,
                host?.hostname,
                host?.primary_alias,
                s.ingress_addr,
            ]
                .filter(Boolean)
                .join(" ")
                .toLowerCase();
            return hay.includes(q);
        });
    }, [sessions, query, hostsByID]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <FilterToolbar
                search={{
                    value: query,
                    onChange: setQuery,
                    placeholder: "Search session, host, user",
                    minWidth: 280,
                }}
                filters={
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={filter}
                        onValueChange={(v) => {
                            if (v) setFilter(v as FilterMode);
                        }}
                    >
                        <ToggleGroupItem value="live">Live</ToggleGroupItem>
                        <ToggleGroupItem value="all">All</ToggleGroupItem>
                    </ToggleGroup>
                }
                count={
                    sessions === null
                        ? "Loading…"
                        : `${sessions.length} ${filter === "live" ? "live" : "total"}`
                }
                refreshLoading={loading}
                onRefresh={refresh}
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
                        {humanizeError(error)}
                    </div>
                )}
                {!sessions && (
                    <div className="flex items-center justify-center p-20">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                )}
                {sessions && sessions.length === 0 && (
                    <EmptyState
                        icon={<ShieldCheck className="size-5" />}
                        title={filter === "live" ? "No live sessions" : "No sessions yet"}
                        description={
                            filter === "live"
                                ? "Switch to All to see closed sessions, or wait for an agent to connect."
                                : "Sessions appear here when an enrolled agent opens a connection."
                        }
                    />
                )}
                {filtered && filtered.length === 0 && sessions && sessions.length > 0 && (
                    <EmptyState
                        title="No matches"
                        description={`No session matches "${query}".`}
                    />
                )}
                {filtered && filtered.length > 0 && (
                    <Card padding={0}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead className="w-[180px]">Session</TableHead>
                                    <TableHead>Host</TableHead>
                                    <TableHead>Ingress</TableHead>
                                    <TableHead className="w-[120px]">User</TableHead>
                                    <TableHead className="w-[140px]">Connected</TableHead>
                                    <TableHead className="w-[180px]">Status</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {filtered.map((s) => {
                                    const host = hostsByID[s.host_id];
                                    const primary =
                                        host?.primary_alias ||
                                        host?.hostname ||
                                        host?.machine_id?.slice(0, 8) ||
                                        "—";
                                    return (
                                        <TableRow
                                            key={s.id}
                                            className="cursor-pointer"
                                            onClick={() =>
                                                navigate(
                                                    `/projects/${project.slug}/hosts/${s.host_id}/files`,
                                                )
                                            }
                                        >
                                            <TableCell>
                                                <Mono>{`${s.id.slice(0, 16)}…`}</Mono>
                                            </TableCell>
                                            <TableCell>
                                                {host ? (
                                                    <span className="text-text-primary">
                                                        {primary}
                                                    </span>
                                                ) : (
                                                    <Mono>{`${s.host_id.slice(0, 8)}…`}</Mono>
                                                )}
                                            </TableCell>
                                            <TableCell>
                                                {s.ingress_addr ? (
                                                    <Mono>{s.ingress_addr}</Mono>
                                                ) : (
                                                    "—"
                                                )}
                                            </TableCell>
                                            <TableCell>
                                                {s.user ? (
                                                    s.user === "root" ? (
                                                        <StatusPill tone="danger">root</StatusPill>
                                                    ) : (
                                                        <Mono>{s.user}</Mono>
                                                    )
                                                ) : (
                                                    "—"
                                                )}
                                            </TableCell>
                                            <TableCell className="text-text-secondary">
                                                {fromNow(s.connected_at)}
                                            </TableCell>
                                            <TableCell>
                                                {s.disconnected_at ? (
                                                    <StatusPill tone="neutral">
                                                        {`closed ${fromNow(s.disconnected_at)}`}
                                                    </StatusPill>
                                                ) : (
                                                    <StatusPill tone="success">live</StatusPill>
                                                )}
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
