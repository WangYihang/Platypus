import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Spin, Table, Tabs } from "antd";
import { ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import StatusDot from "../components/StatusDot";
import StatusPill from "../components/StatusPill";
import PageHeader from "../components/PageHeader";
import { palette, space } from "../layout/theme";
import { Host, Listener, SessionRow, getHost, listHostSessions, listListeners } from "../lib/api";
import { NotifyEvent, SessionEventPayload, onNotify } from "../lib/notify";
import { fromNow, isOnline } from "../lib/time";
import FilesTab from "./host/FilesTab";
import TerminalTab from "./host/TerminalTab";
import TunnelsTab from "./host/TunnelsTab";

interface Props {
    projectID: string;
    hostID: string;
}

// HostView is the main-panel view when a Host is selected in the
// sidebar. Five tabs (Terminal, Files, Tunnels, Sessions, Info) live
// under the page header — Vercel-style underline tab bar shares the
// header surface so the page reads as one stacked unit.
export default function HostView({ projectID, hostID }: Props) {
    const [host, setHost] = useState<Host | null>(null);
    const [sessions, setSessions] = useState<SessionRow[]>([]);
    const [listenersMap, setListenersMap] = useState<Record<string, Listener>>({});
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    // pickedSessionID drives which session Terminal / Files / Tunnels
    // operate on. Lifted here so all three tabs stay in sync and the
    // "my session just disappeared, fall back to the next live one"
    // effect runs once rather than in each tab.
    const [pickedSessionID, setPickedSessionID] = useState<string | null>(null);
    const [activeTab, setActiveTab] = useState<string>("terminal");

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const [h, s, ls] = await Promise.all([
                getHost(projectID, hostID),
                listHostSessions(projectID, hostID),
                listListeners(projectID),
            ]);
            setHost(h);
            setSessions(s);
            const map: Record<string, Listener> = {};
            for (const l of ls) map[l.id] = l;
            setListenersMap(map);
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [projectID, hostID]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const refetchSessions = useCallback(async () => {
        try {
            setSessions(await listHostSessions(projectID, hostID));
        } catch {
            // ignored; the next explicit refresh will recover
        }
    }, [projectID, hostID]);

    useEffect(() => {
        const matches = (p: SessionEventPayload) =>
            p?.host_id === hostID && p?.project_id === projectID;
        const offs: Array<() => void> = [];
        offs.push(
            onNotify(NotifyEvent.SessionOpened, (data) => {
                if (matches(data as SessionEventPayload)) void refetchSessions();
            }),
        );
        offs.push(
            onNotify(NotifyEvent.SessionClosed, (data) => {
                if (matches(data as SessionEventPayload)) void refetchSessions();
            }),
        );
        return () => offs.forEach((off) => off());
    }, [projectID, hostID, refetchSessions]);

    useEffect(() => {
        const live = sessions.filter((s) => !s.disconnected_at);
        if (live.length === 0) {
            if (pickedSessionID !== null) setPickedSessionID(null);
            return;
        }
        if (!pickedSessionID || !live.some((s) => s.id === pickedSessionID)) {
            setPickedSessionID(live[0].id);
        }
    }, [sessions, pickedSessionID]);

    if (loading && !host) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                <Spin />
            </div>
        );
    }
    if (error && !host) {
        return (
            <div style={{ padding: space[5] }}>
                <Alert type="error" message={error} />
            </div>
        );
    }
    if (!host) return null;

    const primary =
        host.primary_alias || host.hostname || host.machine_id?.slice(0, 8) || "unknown";
    const online = isOnline(host.last_seen_at);
    const liveSessions = sessions.filter((s) => !s.disconnected_at);
    const liveCount = liveSessions.length;

    const tabBar = (
        <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            tabBarStyle={{ margin: 0, borderBottom: `1px solid ${palette.border}` }}
            items={[
                { key: "terminal", label: "Terminal" },
                { key: "files", label: "Files" },
                { key: "tunnels", label: "Tunnels" },
                { key: "sessions", label: `Sessions (${sessions.length})` },
                { key: "info", label: "Info" },
            ]}
        />
    );

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title={
                    <span style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                        <StatusDot status={online ? "online" : "offline"} />
                        <span>{primary}</span>
                    </span>
                }
                subtitle={
                    <span>
                        <Mono size={12} color={palette.textSecondary}>
                            {liveCount}
                        </Mono>{" "}
                        active · {host.os || "unknown OS"}
                        {host.fingerprint_fallback && " · fp-fallback"}
                    </span>
                }
                actions={
                    <Button
                        icon={<ReloadOutlined />}
                        loading={loading}
                        onClick={refresh}
                        size="small"
                    >
                        Refresh
                    </Button>
                }
                tabs={tabBar}
            />
            <div
                style={{
                    flex: 1,
                    overflow: "auto",
                    padding: space[6],
                }}
            >
                <div style={{ display: activeTab === "terminal" ? "block" : "none", height: "100%" }}>
                    <TerminalTab
                        liveSessions={liveSessions}
                        picked={pickedSessionID}
                        onPick={setPickedSessionID}
                    />
                </div>
                <div style={{ display: activeTab === "files" ? "block" : "none" }}>
                    {pickedSessionID ? (
                        <FilesTab sessionHash={pickedSessionID} />
                    ) : (
                        <NoLiveSessionNote />
                    )}
                </div>
                <div style={{ display: activeTab === "tunnels" ? "block" : "none" }}>
                    {pickedSessionID ? (
                        <TunnelsTab sessionHash={pickedSessionID} />
                    ) : (
                        <NoLiveSessionNote />
                    )}
                </div>
                <div style={{ display: activeTab === "sessions" ? "block" : "none" }}>
                    <SessionsPanel sessions={sessions} listenersMap={listenersMap} />
                </div>
                <div style={{ display: activeTab === "info" ? "block" : "none" }}>
                    <InfoPanel host={host} />
                </div>
            </div>
        </div>
    );
}

function NoLiveSessionNote() {
    return (
        <EmptyState
            title="No live session"
            description="Start or reconnect an agent to use this tab."
        />
    );
}

function InfoPanel({ host }: { host: Host }) {
    return (
        <div style={{ maxWidth: 720 }}>
            <Card header="Host info" padding={5}>
                <DataList
                    items={[
                        { label: "hostname", value: host.hostname || "—" },
                        { label: "primary alias", value: host.primary_alias || "—" },
                        { label: "OS", value: host.os || "—" },
                        {
                            label: "machine_id",
                            value: host.machine_id ? (
                                <Mono>{host.machine_id}</Mono>
                            ) : (
                                <StatusPill tone="warning">fingerprint fallback</StatusPill>
                            ),
                        },
                        {
                            label: "fingerprint",
                            value: <Mono size={11}>{host.fingerprint}</Mono>,
                        },
                        { label: "first seen", value: fromNow(host.first_seen_at) },
                        { label: "last seen", value: fromNow(host.last_seen_at) },
                    ]}
                />
            </Card>
        </div>
    );
}

function SessionsPanel({
    sessions,
    listenersMap,
}: {
    sessions: SessionRow[];
    listenersMap: Record<string, Listener>;
}) {
    const columns: ColumnsType<SessionRow> = [
        {
            title: "Session",
            dataIndex: "id",
            render: (v: string) => <Mono>{v.slice(0, 16)}…</Mono>,
            width: 180,
        },
        {
            title: "Listener",
            dataIndex: "listener_id",
            render: (id: string) => {
                const l = listenersMap[id];
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
        },
        {
            title: "Remote",
            dataIndex: "remote_addr",
            render: (v?: string) => (v ? <Mono>{v}</Mono> : "—"),
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
        <Card padding={0}>
            <Table
                rowKey="id"
                size="small"
                bordered={false}
                columns={columns}
                dataSource={sessions}
                pagination={{ pageSize: 20, showSizeChanger: false }}
                locale={{
                    emptyText: (
                        <EmptyState
                            title="No sessions"
                            description="No connections recorded for this host yet."
                        />
                    ),
                }}
            />
        </Card>
    );
}
