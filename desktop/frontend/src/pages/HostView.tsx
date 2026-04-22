import { useCallback, useEffect, useState } from "react";
import { Loader2, RotateCw } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import StatusDot from "../components/StatusDot";
import StatusPill from "../components/StatusPill";
import PageHeader from "../components/PageHeader";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import {
    Host,
    Listener,
    SessionRow,
    getHost,
    listHostSessions,
    listListeners,
} from "../lib/api";
import { NotifyEvent, SessionEventPayload, onNotify } from "../lib/notify";
import { fromNow, isOnline } from "../lib/time";
import FilesTab from "./host/FilesTab";
import TerminalTab from "./host/TerminalTab";
import TunnelsTab from "./host/TunnelsTab";

import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

interface Props {
    projectID: string;
    hostID: string;
}

const TABS = ["terminal", "files", "tunnels", "sessions", "info"] as const;
type TabKey = (typeof TABS)[number];

// HostView is the main-panel view when a Host is selected. Five tabs
// (Terminal, Files, Tunnels, Sessions, Info) live under the page header
// — shadcn Tabs for the bar, but the panels render ourselves so the
// underlying tab components (Terminal with persistent xterm state) can
// mount once and stay alive across tab switches.
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

    const project = useCurrentProject();
    const navigate = useNavigate();
    const { tab: tabParam } = useParams<{ tab?: string }>();
    const activeTab: TabKey = (TABS as readonly string[]).includes(tabParam ?? "")
        ? (tabParam as TabKey)
        : "terminal";
    const setActiveTab = (key: string) =>
        navigate(`/projects/${project.slug}/hosts/${hostID}/${key}`);

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
            <div className="flex items-center justify-center p-20">
                <Loader2 className="size-5 animate-spin text-text-muted" />
            </div>
        );
    }
    if (error && !host) {
        return (
            <div style={{ padding: space[5] }}>
                <div
                    style={{
                        padding: `${space[3]}px ${space[4]}px`,
                        border: `1px solid ${palette.danger}`,
                        borderRadius: 6,
                        color: palette.danger,
                        fontSize: 13,
                    }}
                >
                    {error}
                </div>
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
        <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList className="h-9">
                <TabsTrigger value="terminal">Terminal</TabsTrigger>
                <TabsTrigger value="files">Files</TabsTrigger>
                <TabsTrigger value="tunnels">Tunnels</TabsTrigger>
                <TabsTrigger value="sessions">Sessions ({sessions.length})</TabsTrigger>
                <TabsTrigger value="info">Info</TabsTrigger>
            </TabsList>
        </Tabs>
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
                    <Button size="sm" variant="outline" disabled={loading} onClick={refresh}>
                        {loading ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <RotateCw className="size-3.5" />
                        )}
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
                {/* Each tab panel stays mounted (via display:none) so xterm
                    instances in TerminalTab don't get re-created on tab
                    switch — the persistent websocket / scrollback state is
                    expensive to rebuild. */}
                <div
                    style={{
                        display: activeTab === "terminal" ? "block" : "none",
                        height: "100%",
                    }}
                >
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
    if (sessions.length === 0) {
        return (
            <Card padding={0}>
                <EmptyState
                    title="No sessions"
                    description="No connections recorded for this host yet."
                />
            </Card>
        );
    }
    return (
        <Card padding={0}>
            <Table>
                <TableHeader>
                    <TableRow>
                        <TableHead className="w-[180px]">Session</TableHead>
                        <TableHead>Listener</TableHead>
                        <TableHead>User</TableHead>
                        <TableHead>Remote</TableHead>
                        <TableHead className="w-[140px]">Connected</TableHead>
                        <TableHead className="w-[180px]">Status</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {sessions.map((r) => {
                        const l = listenersMap[r.listener_id];
                        return (
                            <TableRow key={r.id}>
                                <TableCell>
                                    <Mono>{`${r.id.slice(0, 16)}…`}</Mono>
                                </TableCell>
                                <TableCell>
                                    {l ? (
                                        <Mono>{`${l.host}:${l.port}`}</Mono>
                                    ) : (
                                        <Mono>{`${r.listener_id.slice(0, 8)}…`}</Mono>
                                    )}
                                </TableCell>
                                <TableCell>
                                    {r.user ? (
                                        r.user === "root" ? (
                                            <StatusPill tone="danger">root</StatusPill>
                                        ) : (
                                            <Mono>{r.user}</Mono>
                                        )
                                    ) : (
                                        "—"
                                    )}
                                </TableCell>
                                <TableCell>
                                    {r.remote_addr ? <Mono>{r.remote_addr}</Mono> : "—"}
                                </TableCell>
                                <TableCell className="text-text-secondary">
                                    {fromNow(r.connected_at)}
                                </TableCell>
                                <TableCell>
                                    {r.disconnected_at ? (
                                        <StatusPill tone="neutral">
                                            {`closed ${fromNow(r.disconnected_at)}`}
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
    );
}
