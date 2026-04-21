import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Descriptions, Spin, Table, Tabs, Tag } from "antd";
import { ReloadOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";

import MainHeader from "../layout/MainHeader";
import { palette } from "../layout/theme";
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
// sidebar. In the final design this hosts sub-tabs for Terminal,
// Files, Tunnels, Sessions, Info. Terminal/Files/Tunnels require
// runtime session integration (xterm WebSocket, file dialogs) that
// depends on either Wails bindings or the web platform shim — both
// of which are wired up for specific session hashes.
//
// This first cut implements the read-only tabs: Info (metadata about
// the host) and Sessions (live + historical connection table).
// Terminal integration follows in a later pass so the live accept
// path can be exercised end-to-end.
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

    // Only refetch sessions on session events — host metadata doesn't
    // change mid-session. A lightweight reload instead of the full
    // refresh above so presence / last_seen don't flicker.
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

    // Keep pickedSessionID pointing at a currently-live session. Runs
    // after every sessions refetch (open / close) to cover:
    //   - initial render (pick the first live session)
    //   - previously-picked session just closed (move to the next)
    //   - all sessions gone (null it out; empty states take over)
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
            <div style={{ padding: 20 }}>
                <Alert type="error" message={error} />
            </div>
        );
    }
    if (!host) return null;

    const primary = host.primary_alias || host.hostname || host.machine_id?.slice(0, 8) || "unknown";
    const online = isOnline(host.last_seen_at);
    const liveSessions = sessions.filter((s) => !s.disconnected_at);
    const liveCount = liveSessions.length;

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <MainHeader
                title={
                    <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <span
                            style={{
                                width: 8,
                                height: 8,
                                borderRadius: "50%",
                                background: online ? palette.success : palette.textSecondary,
                                opacity: online ? 1 : 0.5,
                            }}
                        />
                        <span>{primary}</span>
                    </span>
                }
                subtitle={`${liveCount} active session(s) · ${host.os || "unknown OS"}${host.fingerprint_fallback ? " · fp-fallback" : ""}`}
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
            />
            <div style={{ flex: 1, overflow: "hidden", padding: 20, paddingBottom: 0 }}>
                <Tabs
                    defaultActiveKey="terminal"
                    style={{ height: "100%" }}
                    // forceRender keeps every panel mounted after its
                    // first visit, so switching tabs preserves xterm
                    // scrollback and Files/Tunnels form state. The
                    // trade-off is a slightly heavier DOM tree — fine
                    // at the fleet sizes this targets.
                    items={[
                        {
                            key: "terminal",
                            label: "Terminal",
                            forceRender: true,
                            children: (
                                <TerminalTab
                                    liveSessions={liveSessions}
                                    picked={pickedSessionID}
                                    onPick={setPickedSessionID}
                                />
                            ),
                        },
                        {
                            key: "files",
                            label: "Files",
                            forceRender: true,
                            children: pickedSessionID ? (
                                <FilesTab sessionHash={pickedSessionID} />
                            ) : (
                                <NoLiveSessionNote />
                            ),
                        },
                        {
                            key: "tunnels",
                            label: "Tunnels",
                            forceRender: true,
                            children: pickedSessionID ? (
                                <TunnelsTab sessionHash={pickedSessionID} />
                            ) : (
                                <NoLiveSessionNote />
                            ),
                        },
                        {
                            key: "sessions",
                            label: `Sessions (${sessions.length})`,
                            children: (
                                <SessionsPanel sessions={sessions} listenersMap={listenersMap} />
                            ),
                        },
                        {
                            key: "info",
                            label: "Info",
                            children: <InfoPanel host={host} />,
                        },
                    ]}
                />
            </div>
        </div>
    );
}

function NoLiveSessionNote() {
    return (
        <div style={{ padding: 32 }}>
            <Alert
                type="info"
                showIcon
                message="No live session for this host"
                description="Start or reconnect an agent to use this tab."
            />
        </div>
    );
}

function InfoPanel({ host }: { host: Host }) {
    return (
        <Descriptions
            size="small"
            column={1}
            bordered
            styles={{ label: { width: 180, color: palette.textSecondary } }}
        >
            <Descriptions.Item label="hostname">{host.hostname || "—"}</Descriptions.Item>
            <Descriptions.Item label="primary alias">
                {host.primary_alias || "—"}
            </Descriptions.Item>
            <Descriptions.Item label="OS">{host.os || "—"}</Descriptions.Item>
            <Descriptions.Item label="machine_id">
                {host.machine_id ? (
                    <code style={{ color: palette.textPrimary }}>{host.machine_id}</code>
                ) : (
                    <Tag color="warning">fingerprint fallback</Tag>
                )}
            </Descriptions.Item>
            <Descriptions.Item label="fingerprint">
                <code style={{ color: palette.textSecondary, fontSize: 11 }}>
                    {host.fingerprint}
                </code>
            </Descriptions.Item>
            <Descriptions.Item label="first seen">
                {fromNow(host.first_seen_at)}
            </Descriptions.Item>
            <Descriptions.Item label="last seen">
                {fromNow(host.last_seen_at)}
            </Descriptions.Item>
        </Descriptions>
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
            render: (v: string) => (
                <code style={{ fontSize: 11 }}>{v.slice(0, 16)}…</code>
            ),
            width: 180,
        },
        {
            title: "Listener",
            dataIndex: "listener_id",
            render: (id: string) => {
                const l = listenersMap[id];
                return l ? `${l.host}:${l.port}` : <code style={{ fontSize: 11 }}>{id.slice(0, 8)}…</code>;
            },
        },
        {
            title: "User",
            dataIndex: "user",
            render: (v?: string) => (v ? <Tag color={v === "root" ? "red" : undefined}>{v}</Tag> : "—"),
        },
        {
            title: "Remote",
            dataIndex: "remote_addr",
            render: (v?: string) => v || "—",
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
                    <Tag>{`closed ${fromNow(r.disconnected_at)}`}</Tag>
                ) : (
                    <Tag color="success">live</Tag>
                ),
            width: 180,
        },
    ];
    return (
        <Table
            rowKey="id"
            size="small"
            columns={columns}
            dataSource={sessions}
            pagination={{ pageSize: 20 }}
            locale={{ emptyText: "No sessions yet for this host." }}
        />
    );
}
