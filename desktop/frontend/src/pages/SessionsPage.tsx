import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Button, Input, Segmented, Spin, Table, message } from "antd";
import {
    ReloadOutlined,
    SafetyOutlined,
    SearchOutlined,
} from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
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
    Listener,
    SessionRow,
    listHosts,
    listListeners,
    listProjectSessions,
} from "../lib/api";
import { fromNow } from "../lib/time";

type FilterMode = "live" | "all";

// SessionsPage is the cross-host project sessions list at
// /projects/:slug/sessions. Live + historical view with a Segmented
// filter. Row click → host's terminal tab. Backed by the new
// /api/v1/projects/:pid/sessions endpoint added in step 1.
export default function SessionsPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const [sessions, setSessions] = useState<SessionRow[] | null>(null);
    const [hostsByID, setHostsByID] = useState<Record<string, Host>>({});
    const [listenersByID, setListenersByID] = useState<Record<string, Listener>>({});
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [filter, setFilter] = useState<FilterMode>("live");
    const [query, setQuery] = useState("");
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const opts = filter === "live" ? { live: true } : {};
            const [s, h, l] = await Promise.all([
                listProjectSessions(project.id, opts),
                listHosts(project.id),
                listListeners(project.id),
            ]);
            setSessions(s);
            const hMap: Record<string, Host> = {};
            for (const x of h) hMap[x.id] = x;
            setHostsByID(hMap);
            const lMap: Record<string, Listener> = {};
            for (const x of l) lMap[x.id] = x;
            setListenersByID(lMap);
            setError(null);
        } catch (e) {
            setError(String(e));
            messageApi.error(`load sessions: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [project.id, filter, messageApi]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const filtered = useMemo(() => {
        if (!sessions) return null;
        const q = query.trim().toLowerCase();
        if (!q) return sessions;
        return sessions.filter((s) => {
            const host = hostsByID[s.host_id];
            const lis = listenersByID[s.listener_id];
            const hay = [
                s.id,
                s.user,
                s.remote_addr,
                host?.hostname,
                host?.primary_alias,
                lis ? `${lis.host}:${lis.port}` : undefined,
            ]
                .filter(Boolean)
                .join(" ")
                .toLowerCase();
            return hay.includes(q);
        });
    }, [sessions, query, hostsByID, listenersByID]);

    const columns: ColumnsType<SessionRow> = [
        {
            title: "Session",
            dataIndex: "id",
            render: (v: string) => <Mono>{v.slice(0, 16)}…</Mono>,
            width: 180,
        },
        {
            title: "Host",
            dataIndex: "host_id",
            render: (hid: string) => {
                const h = hostsByID[hid];
                if (!h) return <Mono>{hid.slice(0, 8)}…</Mono>;
                const primary = h.primary_alias || h.hostname || h.machine_id?.slice(0, 8) || "—";
                return <span style={{ color: palette.textPrimary }}>{primary}</span>;
            },
        },
        {
            title: "Listener",
            dataIndex: "listener_id",
            render: (id: string) => {
                const l = listenersByID[id];
                return l ? <Mono>{`${l.host}:${l.port}`}</Mono> : <Mono>{id.slice(0, 8)}…</Mono>;
            },
        },
        {
            title: "User",
            dataIndex: "user",
            render: (v?: string) =>
                v ? (
                    v === "root" ? (
                        <StatusPill tone="danger">root</StatusPill>
                    ) : (
                        <Mono>{v}</Mono>
                    )
                ) : (
                    "—"
                ),
            width: 120,
        },
        {
            title: "Connected",
            dataIndex: "connected_at",
            render: (v: string) => fromNow(v),
            width: 140,
        },
        {
            title: "Status",
            render: (_, r) =>
                r.disconnected_at ? (
                    <StatusPill tone="neutral">{`closed ${fromNow(r.disconnected_at)}`}</StatusPill>
                ) : (
                    <StatusPill tone="success">live</StatusPill>
                ),
            width: 180,
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title="Sessions"
                subtitle={
                    sessions === null
                        ? "Loading…"
                        : `${sessions.length} ${filter === "live" ? "live" : "total"}`
                }
                actions={
                    <Button
                        size="small"
                        icon={<ReloadOutlined />}
                        loading={loading}
                        onClick={refresh}
                    >
                        Refresh
                    </Button>
                }
            />
            <Toolbar
                left={
                    <>
                        <Segmented
                            options={[
                                { label: "Live", value: "live" },
                                { label: "All", value: "all" },
                            ]}
                            value={filter}
                            onChange={(v) => setFilter(v as FilterMode)}
                        />
                        <Input
                            prefix={<SearchOutlined style={{ color: palette.textMuted }} />}
                            placeholder="Search session, host, user, listener"
                            value={query}
                            onChange={(e) => setQuery(e.target.value)}
                            allowClear
                            style={{ maxWidth: 360 }}
                        />
                    </>
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <Alert type="error" message={error} style={{ marginBottom: space[4] }} />
                )}
                {!sessions && (
                    <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                        <Spin />
                    </div>
                )}
                {sessions && sessions.length === 0 && (
                    <EmptyState
                        icon={<SafetyOutlined />}
                        title={filter === "live" ? "No live sessions" : "No sessions yet"}
                        description={
                            filter === "live"
                                ? "Switch to All to see closed sessions, or wait for an agent to connect."
                                : "Sessions appear here when an agent connects to one of your listeners."
                        }
                    />
                )}
                {filtered && filtered.length === 0 && sessions && sessions.length > 0 && (
                    <EmptyState title="No matches" description={`No session matches "${query}".`} />
                )}
                {filtered && filtered.length > 0 && (
                    <Card padding={0}>
                        <Table
                            rowKey="id"
                            columns={columns}
                            dataSource={filtered}
                            pagination={{ pageSize: 25, showSizeChanger: false, hideOnSinglePage: true }}
                            size="small"
                            bordered={false}
                            onRow={(s) => ({
                                onClick: () =>
                                    navigate(
                                        `/projects/${project.slug}/hosts/${s.host_id}/terminal`,
                                    ),
                                style: { cursor: "pointer" },
                            })}
                        />
                    </Card>
                )}
            </div>
        </div>
    );
}
