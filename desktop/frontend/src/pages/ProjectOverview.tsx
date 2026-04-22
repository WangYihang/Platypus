import { useEffect, useMemo, useState } from "react";
import { Loader2, Plus, RotateCw, Router, Users, Zap } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";

import ActivityFeed, { ActivityItem } from "../components/ActivityFeed";
import BarChartCard, { BarPoint } from "../components/charts/BarChartCard";
import Card from "../components/Card";
import LineChartCard, { LinePoint } from "../components/charts/LineChartCard";
import MetricCard from "../components/MetricCard";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import { palette, space } from "../layout/theme";
import {
    Host,
    Listener,
    Project,
    SessionRow,
    listHosts,
    listListeners,
    listProjectSessions,
} from "../lib/api";
import { fromNow, isOnline } from "../lib/time";

interface Props {
    project: Project;
    onOpenMembers?: () => void;
}

// ProjectOverview is the dashboard at /projects/:slug/overview. Four
// KPI tiles, a 24h sessions line chart, top-hosts bar, listener health
// mini-list, recent activity feed, and a quick-actions row at the
// bottom. The "+ New listener" button lives in PageHeader so it's
// always one click away from the project landing.
export default function ProjectOverview({ project, onOpenMembers }: Props) {
    const navigate = useNavigate();
    const [listeners, setListeners] = useState<Listener[] | null>(null);
    const [hosts, setHosts] = useState<Host[] | null>(null);
    const [sessions24h, setSessions24h] = useState<SessionRow[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);

    async function refresh() {
        setLoading(true);
        try {
            const since = new Date(Date.now() - 24 * 60 * 60 * 1000);
            const [l, h, s] = await Promise.all([
                listListeners(project.id),
                listHosts(project.id),
                listProjectSessions(project.id, { since, limit: 1000 }),
            ]);
            setListeners(l);
            setHosts(h);
            setSessions24h(s);
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }

    useEffect(() => {
        void refresh();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [project.id]);

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
                    navigate(`/projects/${project.slug}/hosts/${s.host_id}/terminal`),
            }));
    }, [sessions24h, hosts, navigate, project.slug]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
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
                        <Button
                            size="sm"
                            variant="outline"
                            disabled={loading}
                            onClick={() => void refresh()}
                        >
                            {loading ? (
                                <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                                <RotateCw className="size-3.5" />
                            )}
                            Refresh
                        </Button>
                        {onOpenMembers && (
                            <Button size="sm" variant="outline" onClick={onOpenMembers}>
                                <Users className="size-3.5" />
                                Members
                            </Button>
                        )}
                        <Button
                            size="sm"
                            onClick={() => navigate(`/projects/${project.slug}/listeners`)}
                        >
                            <Plus className="size-3.5" />
                            New listener
                        </Button>
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

                <div
                    style={{
                        display: "grid",
                        gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
                        gap: space[3],
                        marginBottom: space[6],
                    }}
                >
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
                    <MetricCard label="Listeners" value={listeners?.length ?? "—"} />
                    <MetricCard
                        label="Live sessions"
                        value={liveSessionsCount}
                        accent={liveSessionsCount > 0 ? "success" : "default"}
                    />
                </div>

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
                        data={linePoints}
                    />
                    <BarChartCard title="Top hosts (24h)" data={barPoints} />
                </div>

                <div
                    style={{
                        display: "grid",
                        gridTemplateColumns: "minmax(0, 1fr) minmax(0, 1fr)",
                        gap: space[3],
                        marginBottom: space[6],
                    }}
                >
                    <Card
                        header={
                            <span
                                style={{
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "space-between",
                                }}
                            >
                                <span>Listeners</span>
                                <Button
                                    variant="link"
                                    size="sm"
                                    className="h-auto p-0 text-text-secondary"
                                    onClick={() =>
                                        navigate(`/projects/${project.slug}/listeners`)
                                    }
                                >
                                    Manage all →
                                </Button>
                            </span>
                        }
                        padding={0}
                    >
                        {listeners === null || listeners.length === 0 ? (
                            <div
                                style={{
                                    padding: space[5],
                                    color: palette.textSecondary,
                                    fontSize: 13,
                                }}
                            >
                                {listeners === null
                                    ? "Loading…"
                                    : "No listeners yet — create one to start accepting agent connections."}
                            </div>
                        ) : (
                            <div style={{ display: "flex", flexDirection: "column" }}>
                                {listeners.slice(0, 5).map((l, i) => (
                                    <div
                                        key={l.id}
                                        onClick={() =>
                                            navigate(
                                                `/projects/${project.slug}/listeners/${l.id}`,
                                            )
                                        }
                                        style={{
                                            display: "flex",
                                            alignItems: "center",
                                            justifyContent: "space-between",
                                            padding: `${space[3]}px ${space[5]}px`,
                                            borderTop:
                                                i === 0
                                                    ? "none"
                                                    : `1px solid ${palette.border}`,
                                            cursor: "pointer",
                                            fontSize: 13,
                                        }}
                                    >
                                        <Mono>{`${l.host}:${l.port}`}</Mono>
                                        <StatusPill tone="success">listening</StatusPill>
                                    </div>
                                ))}
                            </div>
                        )}
                    </Card>

                    <Card header="Recent activity">
                        <ActivityFeed
                            items={activity}
                            emptyHint="No sessions in the last 24h."
                        />
                    </Card>
                </div>

                <Card header="Quick actions">
                    <div
                        style={{
                            display: "grid",
                            gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))",
                            gap: space[3],
                        }}
                    >
                        <QuickAction
                            icon={<Router className="size-4" />}
                            title="Create a listener"
                            description="Bind a host:port to start accepting agent connections."
                            onClick={() => navigate(`/projects/${project.slug}/listeners`)}
                        />
                        <QuickAction
                            icon={<Zap className="size-4" />}
                            title="Run dispatch"
                            description="Run a command on every flagged live session."
                            onClick={() => navigate(`/projects/${project.slug}/dispatch`)}
                        />
                        {onOpenMembers && (
                            <QuickAction
                                icon={<Users className="size-4" />}
                                title="Invite members"
                                description="Grant operator or viewer access to other users."
                                onClick={onOpenMembers}
                            />
                        )}
                    </div>
                </Card>
            </div>
        </div>
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
