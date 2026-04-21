import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Button, Input, Table, message } from "antd";
import { DesktopOutlined, ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
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
    const [messageApi, contextHolder] = message.useMessage();

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            setHosts(await listHosts(project.id));
            setError(null);
        } catch (e) {
            setError(String(e));
            messageApi.error(`load hosts: ${String(e)}`);
        } finally {
            setLoading(false);
        }
    }, [project.id, messageApi]);

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

    const columns: ColumnsType<Host> = [
        {
            title: "Host",
            render: (_, h) => {
                const primary =
                    h.primary_alias || h.hostname || h.machine_id?.slice(0, 8) || "unknown";
                return (
                    <div style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                        <StatusDot status={isOnline(h.last_seen_at) ? "online" : "offline"} />
                        <span
                            style={{
                                color: palette.textPrimary,
                                fontWeight: 500,
                            }}
                        >
                            {primary}
                        </span>
                    </div>
                );
            },
        },
        {
            title: "OS",
            dataIndex: "os",
            render: (os?: string) => os || <span style={{ color: palette.textMuted }}>—</span>,
            width: 140,
        },
        {
            title: "Machine ID",
            dataIndex: "machine_id",
            render: (mid?: string) =>
                mid ? <Mono>{mid.slice(0, 12)}…</Mono> : <span style={{ color: palette.textMuted }}>fp</span>,
            width: 160,
        },
        {
            title: "Last seen",
            dataIndex: "last_seen_at",
            render: (t: string) => fromNow(t),
            width: 140,
        },
    ];

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            {contextHolder}
            <PageHeader
                title="Hosts"
                subtitle={
                    hosts === null
                        ? "Loading…"
                        : `${hosts.length} total · ${onlineCount} online`
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
                    <Input
                        prefix={<SearchOutlined style={{ color: palette.textMuted }} />}
                        placeholder="Search hostname, alias, OS"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        allowClear
                        style={{ maxWidth: 360 }}
                    />
                }
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                {error && (
                    <Alert
                        type="error"
                        message={error}
                        style={{ marginBottom: space[4] }}
                    />
                )}
                {hosts && hosts.length === 0 ? (
                    <EmptyState
                        icon={<DesktopOutlined />}
                        title="No hosts yet"
                        description="Hosts register themselves when an agent connects to one of your listeners. Create a listener first, then run the agent on a target machine."
                        action={
                            <Button
                                type="primary"
                                onClick={() => navigate(`/projects/${project.slug}/listeners`)}
                            >
                                Manage listeners
                            </Button>
                        }
                    />
                ) : filtered && filtered.length === 0 ? (
                    <EmptyState
                        title="No matches"
                        description={`No host matches "${query}".`}
                    />
                ) : (
                    <Card padding={0}>
                        <Table
                            rowKey="id"
                            columns={columns}
                            dataSource={filtered ?? []}
                            loading={loading && !hosts}
                            pagination={false}
                            size="small"
                            bordered={false}
                            onRow={(h) => ({
                                onClick: () =>
                                    navigate(
                                        `/projects/${project.slug}/hosts/${h.id}/terminal`,
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
