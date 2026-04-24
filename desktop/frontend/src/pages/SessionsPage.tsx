import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, RotateCw, Search, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    Host,
    SessionRow,
    listHosts,
    listProjectSessions,
} from "../lib/api";
import { fromNow } from "../lib/time";

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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

type FilterMode = "live" | "all";

// SessionsPage is the cross-host project sessions list at
// /projects/:slug/sessions. Live + historical view with a toggle
// filter and free-text search over id/user/remote/host/ingress. Row
// click jumps to the host's terminal tab. Data is server-paginated
// (one unbounded fetch — sessions typically number in the dozens),
// so we filter client-side rather than round-tripping each keystroke.
export default function SessionsPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const [sessions, setSessions] = useState<SessionRow[] | null>(null);
    const [hostsByID, setHostsByID] = useState<Record<string, Host>>({});
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<FilterMode>("live");
    const [query, setQuery] = useState("");

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const opts = filter === "live" ? { live: true } : {};
            const [s, h] = await Promise.all([
                listProjectSessions(project.id, opts),
                listHosts(project.id),
            ]);
            setSessions(s);
            const hMap: Record<string, Host> = {};
            for (const x of h) hMap[x.id] = x;
            setHostsByID(hMap);
            setError(null);
        } catch (e) {
            setError(String(e));
            toast.error(`load sessions: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [project.id, filter]);

    useEffect(() => {
        refresh();
    }, [refresh]);

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
            <PageHeader
                title="Sessions"
                subtitle={
                    sessions === null
                        ? "Loading…"
                        : `${sessions.length} ${filter === "live" ? "live" : "total"}`
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
                    <>
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
                        <div className="relative max-w-[360px] w-full">
                            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-text-muted pointer-events-none" />
                            <Input
                                placeholder="Search session, host, user, listener"
                                value={query}
                                onChange={(e) => setQuery(e.target.value)}
                                className="h-8 pl-8"
                            />
                        </div>
                    </>
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
                                : "Sessions appear here when an agent connects to one of your listeners."
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
                                                    `/projects/${project.slug}/hosts/${s.host_id}/terminal`,
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
