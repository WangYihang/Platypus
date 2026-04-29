import { useMemo } from "react";
import { Monitor, Users } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useQueryClient, useQuery } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";

import ActivityFeed, { ActivityItem } from "../components/ActivityFeed";
import AutoGrid from "../components/AutoGrid";
import BarChartCard, { BarPoint } from "../components/charts/BarChartCard";
import Card from "../components/Card";
import { Skeleton } from "@/components/ui/skeleton";
import LineChartCard, { LinePoint } from "../components/charts/LineChartCard";
import MetricCard from "../components/MetricCard";
import Mono from "../components/Mono";
import PageShell from "../components/PageShell";
import RefreshButton from "../components/RefreshButton";
import { palette, space } from "../layout/theme";
import {
    Host,
    Project,
    SessionRow,
    getServerInfo,
    listHosts,
    listProjectSessions,
} from "../lib/api";
import { qk } from "../lib/queryKeys";
import { fromNow, isOnline } from "../lib/time";

interface Props {
    project: Project;
    onOpenMembers?: () => void;
}

// ProjectOverview is the dashboard at /projects/:slug/overview. Four
// KPI tiles (hosts / online / ingress / live sessions), a 24h sessions
// line chart, top-hosts bar, recent activity feed, and a quick-actions
// row at the bottom. The unified ingress means there's only one
// public endpoint agents dial, so the old per-listener mini-list has
// been retired.
export default function ProjectOverview({ project, onOpenMembers }: Props) {
    const navigate = useNavigate();
    const queryClient = useQueryClient();

    const serverInfoQuery = useQuery({
        queryKey: qk.serverInfo(),
        queryFn: () => getServerInfo(),
    });
    const hostsQuery = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });
    // 24h sessions feed for the chart + activity list. The cache key
    // pins `since=24h` symbolically so a refetch always resolves the
    // same window even though the timestamp moves; an exact-second
    // key would defeat caching across the page's three readers.
    const sessions24hQuery = useQuery({
        queryKey: ["projectSessions", project.id, "since-24h"] as const,
        queryFn: () => {
            const since = new Date(Date.now() - 24 * 60 * 60 * 1000);
            return listProjectSessions(project.id, { since, limit: 1000 });
        },
    });

    const publicAddr = serverInfoQuery.data?.public_addr || "";
    const hosts: Host[] | null = hostsQuery.data ?? null;
    const sessions24h: SessionRow[] | null = sessions24hQuery.data ?? null;
    const loading =
        serverInfoQuery.isFetching ||
        hostsQuery.isFetching ||
        sessions24hQuery.isFetching;
    const error =
        serverInfoQuery.error ?? hostsQuery.error ?? sessions24hQuery.error ?? null;

    function refresh() {
        queryClient.invalidateQueries({ queryKey: qk.serverInfo() });
        queryClient.invalidateQueries({ queryKey: qk.hosts(project.id) });
        queryClient.invalidateQueries({
            queryKey: ["projectSessions", project.id, "since-24h"],
        });
    }

    const onlineCount = hosts?.filter((h) => isOnline(h.last_seen_at)).length ?? 0;
    const liveSessionsCount = sessions24h?.filter((s) => !s.disconnected_at).length ?? 0;

    const linePoints = useMemo<LinePoint[]>(
        () => bucketSessionsByHour(sessions24h ?? []),
        [sessions24h],
    );

    const barPoints = useMemo<BarPoint[]>(() => {
        if (!sessions24h || !hosts) return [];
        const counts: Record<string, number> = {};
        for (const s of sessions24h) {
            counts[s.host_id] = (counts[s.host_id] || 0) + 1;
        }
        const labelOf = (id: string) => {
            const h = hosts.find((x) => x.id === id);
            return (
                h?.primary_alias ||
                h?.hostname ||
                h?.machine_id?.slice(0, 8) ||
                id.slice(0, 8)
            );
        };
        return Object.entries(counts)
            .map(([id, n]) => ({ label: labelOf(id), value: n }))
            .sort((a, b) => b.value - a.value)
            .slice(0, 5);
    }, [sessions24h, hosts]);

    const activity = useMemo<ActivityItem[]>(() => {
        if (!sessions24h || !hosts) return [];
        const hostLabel = (id: string) => {
            const h = hosts.find((x) => x.id === id);
            return h?.primary_alias || h?.hostname || h?.machine_id?.slice(0, 8) || id.slice(0, 8);
        };
        return sessions24h
            .slice(0, 10)
            .map((s) => ({
                id: s.id,
                when: fromNow(s.connected_at),
                status: s.disconnected_at
                    ? ("offline" as const)
                    : ("online" as const),
                actor: <Mono size={12}>{s.id.slice(0, 8)}</Mono>,
                verb: s.disconnected_at ? "closed on" : "opened on",
                target: <span style={{ color: palette.textPrimary }}>{hostLabel(s.host_id)}</span>,
                onClick: () =>
                    navigate(`/projects/${project.slug}/hosts/${s.host_id}/files`),
            }));
    }, [sessions24h, hosts, navigate, project.slug]);

    return (
        <PageShell
            title={project.name}
            subtitle={
                <>
                    <Mono size={12} color={palette.textMuted}>
                        {project.slug}
                    </Mono>{" "}
                    · overview
                </>
            }
            actions={
                <>
                    <RefreshButton loading={loading} onClick={() => void refresh()} />
                    {onOpenMembers && (
                        <Button size="sm" variant="outline" onClick={onOpenMembers}>
                            <Users className="size-3.5" />
                            Members
                        </Button>
                    )}
                </>
            }
            bodyPadding={8}
        >
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
                        {String(error)}
                    </div>
                )}

                {hosts === null || sessions24h === null ? (
                    <ProjectOverviewSkeleton />
                ) : (
                <>
                <AutoGrid minSize={220} gap={3} style={{ marginBottom: space[6] }}>
                    <MetricCard label="Hosts" value={hosts?.length ?? "—"} />
                    <MetricCard
                        label="Online now"
                        value={onlineCount}
                        accent={onlineCount > 0 ? "success" : "default"}
                        hint={
                            hosts && hosts.length > 0
                                ? `${onlineCount} of ${hosts.length} hosts reachable`
                                : undefined
                        }
                    />
                    {/* Ingress is an address, not a stat — surfaced in
                        the dedicated "Ingress" Card below the chart row
                        instead of a KPI tile. Mixing host:port into the
                        same big-number slot wrecked the grid rhythm. */}
                    <MetricCard
                        label="Live sessions"
                        value={liveSessionsCount}
                        accent={liveSessionsCount > 0 ? "success" : "default"}
                    />
                </AutoGrid>

                <div
                    style={{
                        display: "grid",
                        gridTemplateColumns: "minmax(0, 2fr) minmax(0, 1fr)",
                        gap: space[3],
                        marginBottom: space[6],
                    }}
                >
                    <LineChartCard
                        title="Sessions (last 24h)"
                        hint="New sessions per hour, across every host."
                        seriesLabel="Sessions per hour"
                        data={linePoints}
                    />
                    <BarChartCard
                        title="Top hosts (24h)"
                        seriesLabel="Sessions per host"
                        data={barPoints}
                    />
                </div>

                <div
                    style={{
                        display: "grid",
                        gridTemplateColumns: "minmax(0, 1fr) minmax(0, 1fr)",
                        gap: space[3],
                        marginBottom: space[6],
                    }}
                >
                    <Card header="Ingress" padding={0}>
                        <div
                            style={{
                                padding: space[5],
                                color: palette.textSecondary,
                                fontSize: 13,
                                lineHeight: 1.6,
                            }}
                        >
                            Agents dial a single TLS port (ALPN-multiplexed) and
                            enrol with a project-scoped PAT. The ingress address
                            below is what the install script and mesh bootstrap
                            hand out:
                            <div style={{ marginTop: space[3] }}>
                                <Mono>{publicAddr || "— not reported —"}</Mono>
                            </div>
                        </div>
                    </Card>

                    <Card header="Recent activity">
                        <ActivityFeed
                            items={activity}
                            emptyHint="No sessions in the last 24h."
                        />
                    </Card>
                </div>

                <Card header="Quick actions">
                    <AutoGrid minSize={240} gap={3}>
                        <QuickAction
                            icon={<Monitor className="size-4" />}
                            title="Open Fleet"
                            description="Table, timeline, and topology views of every host in this project."
                            onClick={() => navigate(`/projects/${project.slug}/fleet`)}
                        />
                        {onOpenMembers && (
                            <QuickAction
                                icon={<Users className="size-4" />}
                                title="Invite members"
                                description="Grant operator or viewer access to other users."
                                onClick={onOpenMembers}
                            />
                        )}
                    </AutoGrid>
                </Card>
                </>
                )}
        </PageShell>
    );
}

// ProjectOverviewSkeleton matches the populated layout: a 3-tile KPI
// grid, a 2:1 chart row, a 1:1 ingress / activity row. Same column
// shapes as the loaded view so the loaded ↔ loading transition is a
// content swap with no layout pop. Tagged
// data-testid="overview-skeleton" so the spec can detect the
// loading state without depending on Tailwind class names.
function ProjectOverviewSkeleton() {
    return (
        <>
            <AutoGrid minSize={220} gap={3} style={{ marginBottom: space[6] }}>
                {Array.from({ length: 3 }).map((_, i) => (
                    <Card key={i} padding={5} data-testid="overview-skeleton">
                        <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
                            <Skeleton className="h-3 w-16" />
                            <Skeleton className="h-7 w-20" />
                            <Skeleton className="h-3 w-32" />
                        </div>
                    </Card>
                ))}
            </AutoGrid>
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "minmax(0, 2fr) minmax(0, 1fr)",
                    gap: space[3],
                    marginBottom: space[6],
                }}
            >
                <Card padding={5} data-testid="overview-skeleton">
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Skeleton className="h-4 w-40" />
                        <Skeleton className="h-3 w-56" />
                        <Skeleton className="h-40 w-full" />
                    </div>
                </Card>
                <Card padding={5} data-testid="overview-skeleton">
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-40 w-full" />
                    </div>
                </Card>
            </div>
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "minmax(0, 1fr) minmax(0, 1fr)",
                    gap: space[3],
                    marginBottom: space[6],
                }}
            >
                <Card padding={5} data-testid="overview-skeleton">
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Skeleton className="h-4 w-24" />
                        <Skeleton className="h-3 w-3/4" />
                        <Skeleton className="h-3 w-1/2" />
                    </div>
                </Card>
                <Card padding={5} data-testid="overview-skeleton">
                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-3 w-full" />
                        <Skeleton className="h-3 w-5/6" />
                        <Skeleton className="h-3 w-2/3" />
                    </div>
                </Card>
            </div>
        </>
    );
}

function QuickAction({
    icon,
    title,
    description,
    onClick,
}: {
    icon: React.ReactNode;
    title: string;
    description: string;
    onClick: () => void;
}) {
    return (
        <button
            type="button"
            onClick={onClick}
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[2],
                padding: space[5],
                background: palette.main,
                border: `1px solid ${palette.border}`,
                borderRadius: 6,
                cursor: "pointer",
                textAlign: "left",
                transition: "border-color 120ms ease",
            }}
            onMouseEnter={(e) => (e.currentTarget.style.borderColor = palette.borderStrong)}
            onMouseLeave={(e) => (e.currentTarget.style.borderColor = palette.border)}
        >
            <div style={{ color: palette.textPrimary, fontSize: 16 }}>{icon}</div>
            <div style={{ color: palette.textPrimary, fontWeight: 600, fontSize: 14 }}>
                {title}
            </div>
            <div style={{ color: palette.textSecondary, fontSize: 12, lineHeight: 1.5 }}>
                {description}
            </div>
        </button>
    );
}

// bucketSessionsByHour groups sessions into 24 one-hour buckets, ending
// at "now". Each bucket counts new sessions whose connected_at falls in
// that hour. Drives the time-series line chart on Overview.
function bucketSessionsByHour(sessions: SessionRow[]): LinePoint[] {
    const buckets: LinePoint[] = [];
    const now = new Date();
    now.setMinutes(0, 0, 0);
    for (let i = 23; i >= 0; i--) {
        const hour = new Date(now.getTime() - i * 60 * 60 * 1000);
        const label = `${String(hour.getHours()).padStart(2, "0")}:00`;
        buckets.push({ label, value: 0, ts: hour.getTime() });
    }
    for (const s of sessions) {
        const t = new Date(s.connected_at).getTime();
        const idx = Math.floor((t - buckets[0].ts!) / (60 * 60 * 1000));
        if (idx >= 0 && idx < buckets.length) buckets[idx].value += 1;
    }
    return buckets;
}
