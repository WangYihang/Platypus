import { useCallback, useEffect, useMemo, useState } from "react";
import {
    Alert,
    Button,
    Drawer,
    Input,
    Segmented,
    Select,
    Spin,
    Switch,
    Table,
    Tooltip,
    message,
} from "antd";
import {
    ClockCircleOutlined,
    DownloadOutlined,
    ReloadOutlined,
    SearchOutlined,
} from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import Toolbar from "../components/Toolbar";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    ActivityItem,
    ActivityOutcome,
    exportProjectActivitiesBlob,
    listProjectActivities,
    ListActivitiesOpts,
} from "../lib/api";
import { fromNow } from "../lib/time";

type TimeRange = "24h" | "7d" | "30d" | "all";

// Curated superset of categories the backend writes. Kept static so the
// filter dropdown renders before the first fetch; the underlying `q`
// text search is the escape hatch for anything not captured here.
const CATEGORY_OPTIONS = [
    "auth",
    "session",
    "command",
    "file",
    "tunnel",
    "listener",
    "agent",
    "admin",
    "project",
    "server",
    "user",
];

const OUTCOME_TONE: Record<ActivityOutcome, "success" | "warning" | "danger"> = {
    success: "success",
    denied: "warning",
    error: "danger",
};

// ActivitiesPage renders /projects/:slug/activities: a paginated,
// filterable, project-scoped view of the unified activity log. Clicking
// a row opens a drawer with the full structured meta payload so the
// table can stay compact.
export default function ActivitiesPage() {
    const project = useCurrentProject();
    const [items, setItems] = useState<ActivityItem[] | null>(null);
    const [nextCursor, setNextCursor] = useState<string | null>(null);
    const [total, setTotal] = useState<number | null>(null);
    const [loading, setLoading] = useState(false);
    const [loadingMore, setLoadingMore] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [messageApi, contextHolder] = message.useMessage();

    const [range, setRange] = useState<TimeRange>("7d");
    const [categories, setCategories] = useState<string[]>([]);
    const [actor, setActor] = useState("");
    const [outcome, setOutcome] = useState<ActivityOutcome | "">("");
    const [query, setQuery] = useState("");
    const [includeGlobal, setIncludeGlobal] = useState(false);
    const [selected, setSelected] = useState<ActivityItem | null>(null);

    const fromDate = useMemo(() => rangeToFrom(range), [range]);

    const buildOpts = useCallback(
        (cursor?: string): ListActivitiesOpts => ({
            from: fromDate ?? undefined,
            category: categories.length ? categories : undefined,
            actor: actor.trim() || undefined,
            outcome: outcome || undefined,
            q: query.trim() || undefined,
            includeGlobal,
            limit: 50,
            cursor,
            includeTotal: !cursor, // only fetch total on page 1
        }),
        [fromDate, categories, actor, outcome, query, includeGlobal],
    );

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const resp = await listProjectActivities(project.id, buildOpts());
            setItems(resp.items);
            setNextCursor(resp.next_cursor ?? null);
            setTotal(resp.total ?? null);
            setError(null);
        } catch (e) {
            setError(String(e));
            messageApi.error(`load activities: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [project.id, buildOpts, messageApi]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const loadMore = useCallback(async () => {
        if (!nextCursor) return;
        setLoadingMore(true);
        try {
            const resp = await listProjectActivities(project.id, buildOpts(nextCursor));
            setItems((prev) => [...(prev ?? []), ...resp.items]);
            setNextCursor(resp.next_cursor ?? null);
        } catch (e) {
            messageApi.error(`load more: ${String(e)}`);
        } finally {
            setLoadingMore(false);
        }
    }, [project.id, nextCursor, buildOpts, messageApi]);

    const handleExport = useCallback(
        async (format: "jsonl" | "csv") => {
            try {
                const blob = await exportProjectActivitiesBlob(project.id, {
                    ...buildOpts(),
                    format,
                });
                const url = URL.createObjectURL(blob);
                const a = document.createElement("a");
                a.href = url;
                a.download = `activities-${project.slug}.${format}`;
                a.click();
                URL.revokeObjectURL(url);
            } catch (e) {
                messageApi.error(`export: ${String(e)}`);
            }
        },
        [project.id, project.slug, buildOpts, messageApi],
    );

    const columns: ColumnsType<ActivityItem> = [
        {
            title: "When",
            dataIndex: "at",
            render: (v: string) => (
                <Tooltip title={new Date(v).toLocaleString()}>
                    <span style={{ color: palette.textSecondary }}>{fromNow(v)}</span>
                </Tooltip>
            ),
            width: 140,
        },
        {
            title: "Actor",
            dataIndex: "actor_user",
            render: (_: string, r: ActivityItem) => (
                <div style={{ display: "flex", flexDirection: "column" }}>
                    <span style={{ color: palette.textPrimary }}>
                        {r.actor_user || <span style={{ color: palette.textMuted }}>system</span>}
                    </span>
                    {r.actor_ip && (
                        <Mono style={{ fontSize: 11, color: palette.textMuted }}>
                            {r.actor_ip}
                        </Mono>
                    )}
                </div>
            ),
            width: 180,
        },
        {
            title: "Action",
            dataIndex: "action",
            render: (v: string, r: ActivityItem) => (
                <div style={{ display: "flex", flexDirection: "column" }}>
                    <Mono>{v}</Mono>
                    <span style={{ fontSize: 11, color: palette.textMuted }}>{r.category}</span>
                </div>
            ),
            width: 220,
        },
        {
            title: "Target",
            dataIndex: "target_label",
            render: (_: string, r: ActivityItem) => {
                const label = r.target_label || r.target_id || "—";
                const short = label.length > 40 ? `${label.slice(0, 40)}…` : label;
                return (
                    <Tooltip title={label}>
                        <Mono>{short}</Mono>
                    </Tooltip>
                );
            },
        },
        {
            title: "Outcome",
            dataIndex: "outcome",
            render: (v: ActivityOutcome) => (
                <StatusPill tone={OUTCOME_TONE[v] ?? "neutral"}>{v}</StatusPill>
            ),
            width: 110,
        },
        {
            title: "Duration",
            dataIndex: "duration_ms",
            render: (v?: number) =>
                typeof v === "number" ? (
                    <span style={{ color: palette.textSecondary }}>{formatDuration(v)}</span>
                ) : (
                    <span style={{ color: palette.textMuted }}>—</span>
                ),
            width: 100,
        },
    ];

    const subtitle = useMemo(() => {
        if (items === null) return "Loading…";
        if (total !== null) return `${total.toLocaleString()} events`;
        return `${items.length.toLocaleString()} loaded`;
    }, [items, total]);

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title="Activities"
                subtitle={subtitle}
                actions={
                    <>
                        <Button
                            size="small"
                            icon={<DownloadOutlined />}
                            onClick={() => handleExport("jsonl")}
                        >
                            Export JSONL
                        </Button>
                        <Button
                            size="small"
                            icon={<DownloadOutlined />}
                            onClick={() => handleExport("csv")}
                        >
                            Export CSV
                        </Button>
                        <Button
                            size="small"
                            icon={<ReloadOutlined />}
                            loading={loading}
                            onClick={refresh}
                        >
                            Refresh
                        </Button>
                    </>
                }
            />
            <Toolbar
                left={
                    <>
                        <Segmented
                            options={[
                                { label: "24h", value: "24h" },
                                { label: "7d", value: "7d" },
                                { label: "30d", value: "30d" },
                                { label: "All", value: "all" },
                            ]}
                            value={range}
                            onChange={(v) => setRange(v as TimeRange)}
                        />
                        <Select
                            mode="multiple"
                            allowClear
                            placeholder="Category"
                            style={{ minWidth: 180 }}
                            value={categories}
                            onChange={setCategories}
                            options={CATEGORY_OPTIONS.map((c) => ({ label: c, value: c }))}
                            size="middle"
                        />
                        <Select
                            allowClear
                            placeholder="Outcome"
                            style={{ minWidth: 120 }}
                            value={outcome || undefined}
                            onChange={(v) => setOutcome((v ?? "") as ActivityOutcome | "")}
                            options={[
                                { label: "success", value: "success" },
                                { label: "denied", value: "denied" },
                                { label: "error", value: "error" },
                            ]}
                        />
                        <Input
                            placeholder="Actor (username)"
                            value={actor}
                            onChange={(e) => setActor(e.target.value)}
                            allowClear
                            style={{ maxWidth: 180 }}
                        />
                        <Input
                            prefix={<SearchOutlined style={{ color: palette.textMuted }} />}
                            placeholder="Search action / target / meta"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            allowClear
                            style={{ maxWidth: 260 }}
                        />
                    </>
                }
                right={
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <span style={{ color: palette.textSecondary, fontSize: 12 }}>
                            Include global
                        </span>
                        <Switch size="small" checked={includeGlobal} onChange={setIncludeGlobal} />
                    </div>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <Alert type="error" message={error} style={{ marginBottom: space[4] }} />
                )}
                {!items && (
                    <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                        <Spin />
                    </div>
                )}
                {items && items.length === 0 && (
                    <EmptyState
                        icon={<ClockCircleOutlined />}
                        title="No activities yet"
                        description="As users, agents, and the server do things, their actions appear here in real time."
                    />
                )}
                {items && items.length > 0 && (
                    <Card padding={0}>
                        <Table<ActivityItem>
                            rowKey="id"
                            columns={columns}
                            dataSource={items}
                            pagination={false}
                            size="small"
                            bordered={false}
                            onRow={(it) => ({
                                onClick: () => setSelected(it),
                                style: { cursor: "pointer" },
                            })}
                        />
                        {nextCursor && (
                            <div
                                style={{
                                    padding: space[4],
                                    display: "flex",
                                    justifyContent: "center",
                                }}
                            >
                                <Button loading={loadingMore} onClick={loadMore}>
                                    Load more
                                </Button>
                            </div>
                        )}
                    </Card>
                )}
            </div>
            <ActivityDetailDrawer item={selected} onClose={() => setSelected(null)} />
        </div>
    );
}

// ActivityDetailDrawer shows the full structured payload for one row.
// Kept as a separate component so the table stays compact and the
// drawer can grow richer over time (links to session, request_id
// cross-reference, etc.) without muddying the list.
function ActivityDetailDrawer({
    item,
    onClose,
}: {
    item: ActivityItem | null;
    onClose: () => void;
}) {
    const rows: Array<{ label: string; value: React.ReactNode }> = [];
    if (item) {
        rows.push({ label: "When", value: new Date(item.at).toLocaleString() });
        rows.push({ label: "Action", value: <Mono>{item.action}</Mono> });
        rows.push({ label: "Category", value: <Mono>{item.category}</Mono> });
        rows.push({
            label: "Actor",
            value: (
                <div>
                    <div>{item.actor_user || "(system)"}</div>
                    {item.actor_ip && (
                        <Mono style={{ fontSize: 11, color: palette.textMuted }}>
                            {item.actor_ip}
                        </Mono>
                    )}
                    {item.actor_ua && (
                        <div style={{ fontSize: 11, color: palette.textMuted }}>
                            {item.actor_ua}
                        </div>
                    )}
                </div>
            ),
        });
        rows.push({
            label: "Target",
            value: (
                <div>
                    {item.target_type && (
                        <span style={{ color: palette.textMuted }}>{item.target_type} · </span>
                    )}
                    <Mono>{item.target_label || item.target_id || "—"}</Mono>
                </div>
            ),
        });
        rows.push({
            label: "Outcome",
            value: <StatusPill tone={OUTCOME_TONE[item.outcome] ?? "neutral"}>{item.outcome}</StatusPill>,
        });
        if (item.error) {
            rows.push({ label: "Error", value: <Mono>{item.error}</Mono> });
        }
        if (typeof item.duration_ms === "number") {
            rows.push({ label: "Duration", value: formatDuration(item.duration_ms) });
        }
        if (item.session_id) {
            rows.push({ label: "Session", value: <Mono>{item.session_id}</Mono> });
        }
        if (item.request_id) {
            rows.push({ label: "Request ID", value: <Mono>{item.request_id}</Mono> });
        }
        if (item.project_id) {
            rows.push({ label: "Project", value: <Mono>{item.project_id}</Mono> });
        }
    }

    return (
        <Drawer
            open={item !== null}
            onClose={onClose}
            width={520}
            title={item ? <Mono>{item.action}</Mono> : "Activity"}
            styles={{ body: { padding: space[6] } }}
        >
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "110px 1fr",
                    gap: `${space[3]}px ${space[5]}px`,
                    marginBottom: space[6],
                }}
            >
                {rows.map((r) => (
                    <Fragment key={r.label} label={r.label} value={r.value} />
                ))}
            </div>
            {item?.meta && (
                <div>
                    <div
                        style={{
                            fontSize: 11,
                            textTransform: "uppercase",
                            color: palette.textMuted,
                            marginBottom: space[2],
                        }}
                    >
                        Meta
                    </div>
                    <pre
                        style={{
                            background: palette.surface,
                            border: `1px solid ${palette.border}`,
                            padding: space[4],
                            borderRadius: 4,
                            fontSize: 12,
                            color: palette.textPrimary,
                            overflow: "auto",
                            maxHeight: 360,
                        }}
                    >
                        {typeof item.meta === "string"
                            ? item.meta
                            : JSON.stringify(item.meta, null, 2)}
                    </pre>
                </div>
            )}
        </Drawer>
    );
}

function Fragment({ label, value }: { label: string; value: React.ReactNode }) {
    return (
        <>
            <div
                style={{
                    fontSize: 11,
                    textTransform: "uppercase",
                    color: palette.textMuted,
                    alignSelf: "start",
                    paddingTop: 2,
                }}
            >
                {label}
            </div>
            <div style={{ color: palette.textPrimary, wordBreak: "break-word" }}>{value}</div>
        </>
    );
}

// rangeToFrom maps the segmented control's label to a lower-bound Date.
// "all" → undefined so the server's "no from filter" branch kicks in.
function rangeToFrom(range: TimeRange): Date | null {
    const now = Date.now();
    switch (range) {
        case "24h":
            return new Date(now - 24 * 60 * 60 * 1000);
        case "7d":
            return new Date(now - 7 * 24 * 60 * 60 * 1000);
        case "30d":
            return new Date(now - 30 * 24 * 60 * 60 * 1000);
        case "all":
            return null;
    }
}

// formatDuration renders a millisecond count in the most compact form a
// human can eyeball: <1s stays as ms, 1s-60s as seconds with one
// decimal, above that as minutes+seconds. The UI never shows more
// precision than needed.
function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms} ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
    const mins = Math.floor(ms / 60_000);
    const secs = Math.floor((ms % 60_000) / 1000);
    return `${mins}m ${secs}s`;
}
