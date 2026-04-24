import * as React from "react";
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
    HostSysInfo,
    SessionRow,
    getHost,
    getHostSysInfo,
    listHostSessions,
} from "../lib/api";
import { NotifyEvent, SessionEventPayload, onNotify } from "../lib/notify";
import { fromNow, isOnline } from "../lib/time";
import FilesTab from "./host/FilesTab";
import TerminalTab from "./host/TerminalTab";

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

const TABS = ["terminal", "files", "sessions", "info"] as const;
type TabKey = (typeof TABS)[number];

// HostView is the main-panel view when a Host is selected. Four tabs
// (Terminal, Files, Sessions, Info) live under the page header —
// shadcn Tabs for the bar, but the panels render ourselves so the
// underlying tab components (Terminal with persistent xterm state) can
// mount once and stay alive across tab switches.
export default function HostView({ projectID, hostID }: Props) {
    const [host, setHost] = useState<Host | null>(null);
    const [sessions, setSessions] = useState<SessionRow[]>([]);
    const [sysInfo, setSysInfo] = useState<HostSysInfo | null>(null);
    const [sysInfoError, setSysInfoError] = useState<string | null>(null);
    const [sysInfoLoading, setSysInfoLoading] = useState(false);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    // pickedSessionID drives which session Terminal / Files operate
    // on. Lifted here so both tabs stay in sync and the "my session
    // just disappeared, fall back to the next live one" effect runs
    // once rather than in each tab.
    const [pickedSessionID, setPickedSessionID] = useState<string | null>(null);

    const project = useCurrentProject();
    const navigate = useNavigate();
    const { tab: tabParam } = useParams<{ tab?: string }>();
    const activeTab: TabKey = (TABS as readonly string[]).includes(tabParam ?? "")
        ? (tabParam as TabKey)
        : "terminal";
    const setActiveTab = (key: string) =>
        navigate(`/projects/${project.slug}/hosts/${hostID}/${key}`);

    const refreshSysInfo = useCallback(async () => {
        setSysInfoLoading(true);
        try {
            const s = await getHostSysInfo(projectID, hostID);
            setSysInfo(s);
            setSysInfoError(null);
        } catch (e) {
            // Expected when the agent is offline; surface without
            // clobbering the rest of the page.
            setSysInfo(null);
            setSysInfoError(String(e));
        } finally {
            setSysInfoLoading(false);
        }
    }, [projectID, hostID]);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const [h, s] = await Promise.all([
                getHost(projectID, hostID),
                listHostSessions(projectID, hostID),
            ]);
            setHost(h);
            setSessions(s);
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
        // Fire sysinfo refresh in parallel but don't block the UI
        // on a potentially-offline agent.
        void refreshSysInfo();
    }, [projectID, hostID, refreshSysInfo]);

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
                <div style={{ display: activeTab === "sessions" ? "block" : "none" }}>
                    <SessionsPanel sessions={sessions} />
                </div>
                <div style={{ display: activeTab === "info" ? "block" : "none" }}>
                    <InfoPanel
                        host={host}
                        sysInfo={sysInfo}
                        sysInfoError={sysInfoError}
                        sysInfoLoading={sysInfoLoading}
                        onRefreshSysInfo={refreshSysInfo}
                    />
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

interface InfoPanelProps {
    host: Host;
    sysInfo: HostSysInfo | null;
    sysInfoError: string | null;
    sysInfoLoading: boolean;
    onRefreshSysInfo: () => void;
}

// InfoPanel bundles four cards: Identity (DB-cached host row),
// System (OS / kernel / platform / CPU / memory), Network (primary
// addr, interfaces), and Storage (disk partitions). We prefer the
// live SysInfo fields when available (they include CPU %, memory
// used, etc.) and fall back to the cached Host columns otherwise.
function InfoPanel({ host, sysInfo, sysInfoError, sysInfoLoading, onRefreshSysInfo }: InfoPanelProps) {
    // Prefer live sysInfo values; fall through to DB-cached Host.
    const kernel = sysInfo?.kernel_version || host.kernel_version;
    const platform = sysInfo?.platform || host.platform;
    const platformVersion = sysInfo?.platform_version || host.platform_version;
    const platformFamily = sysInfo?.platform_family || host.platform_family;
    const arch = sysInfo?.arch || host.arch;
    const cpuModel = sysInfo?.cpu_model || host.cpu_model;
    const numCPU = sysInfo?.num_cpu || host.num_cpu;
    const numCPUPhysical = sysInfo?.num_cpu_physical;
    const memTotal = sysInfo?.mem_total || host.mem_total_bytes;
    const currentUser = sysInfo?.current_user || host.current_user;
    const timezone = sysInfo?.timezone || host.timezone;
    const primaryIP = sysInfo?.primary_ip || host.primary_ip;
    const primaryMAC = sysInfo?.primary_mac || host.primary_mac;
    const bootTime = sysInfo?.boot_time_unix || host.boot_time_unix;
    const agentVersion = sysInfo?.agent_version || host.agent_version;

    const liveBadge = sysInfoLoading ? (
        <StatusPill tone="neutral">refreshing…</StatusPill>
    ) : sysInfo ? (
        <StatusPill tone="success">live</StatusPill>
    ) : (
        <StatusPill tone="warning">cached</StatusPill>
    );

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4], maxWidth: 960 }}>
            <Card
                header={
                    <span style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                        <span>Identity</span>
                    </span>
                }
                padding={5}
            >
                <DataList
                    items={[
                        { label: "hostname", value: host.hostname || sysInfo?.hostname || "—" },
                        { label: "primary alias", value: host.primary_alias || "—" },
                        {
                            label: "agent id",
                            value: host.agent_id ? <Mono size={11}>{host.agent_id}</Mono> : "—",
                        },
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

            <Card
                header={
                    <span
                        style={{
                            display: "flex",
                            alignItems: "center",
                            gap: space[2],
                            justifyContent: "space-between",
                            width: "100%",
                        }}
                    >
                        <span style={{ display: "flex", alignItems: "center", gap: space[2] }}>
                            <span>System</span>
                            {liveBadge}
                        </span>
                        <Button size="sm" variant="ghost" onClick={onRefreshSysInfo} disabled={sysInfoLoading}>
                            {sysInfoLoading ? (
                                <Loader2 className="size-3.5 animate-spin" />
                            ) : (
                                <RotateCw className="size-3.5" />
                            )}
                            Refresh
                        </Button>
                    </span>
                }
                padding={5}
            >
                <DataList
                    items={[
                        {
                            label: "OS / arch",
                            value: (
                                <span>
                                    {host.os || sysInfo?.os || "—"}
                                    {arch ? ` · ${arch}` : ""}
                                </span>
                            ),
                        },
                        {
                            label: "platform",
                            value: (
                                <span>
                                    {platform || "—"}
                                    {platformVersion ? ` ${platformVersion}` : ""}
                                    {platformFamily ? ` (${platformFamily})` : ""}
                                </span>
                            ),
                        },
                        { label: "kernel", value: kernel ? <Mono>{kernel}</Mono> : "—" },
                        {
                            label: "virtualization",
                            value: sysInfo?.virtualization ? <Mono>{sysInfo.virtualization}</Mono> : "—",
                        },
                        {
                            label: "CPU",
                            value: (
                                <span>
                                    {cpuModel || "—"}
                                    {numCPU ? ` · ${numCPU}` : ""}
                                    {numCPU ? " logical" : ""}
                                    {numCPUPhysical ? ` / ${numCPUPhysical} physical` : ""}
                                    {sysInfo?.cpu_mhz ? ` · ${Math.round(sysInfo.cpu_mhz)} MHz` : ""}
                                </span>
                            ),
                        },
                        {
                            label: "CPU usage",
                            value:
                                sysInfo?.cpu_percent !== undefined
                                    ? `${sysInfo.cpu_percent.toFixed(1)} %`
                                    : "—",
                        },
                        {
                            label: "memory",
                            value: renderMemoryLine(
                                sysInfo?.mem_used,
                                memTotal,
                                sysInfo?.mem_available,
                            ),
                        },
                        {
                            label: "swap",
                            value: renderMemoryLine(sysInfo?.swap_used, sysInfo?.swap_total),
                        },
                        {
                            label: "load avg",
                            value: renderLoadLine(sysInfo?.load1, sysInfo?.load5, sysInfo?.load15),
                        },
                        {
                            label: "uptime",
                            value: renderUptime(sysInfo?.uptime_seconds, bootTime),
                        },
                        { label: "timezone", value: timezone || "—" },
                        { label: "current user", value: currentUser ? <Mono>{currentUser}</Mono> : "—" },
                        {
                            label: "processes",
                            value: sysInfo?.process_count ? String(sysInfo.process_count) : "—",
                        },
                        { label: "agent version", value: agentVersion ? <Mono>{agentVersion}</Mono> : "—" },
                    ]}
                />
                {sysInfoError && !sysInfo && (
                    <div
                        style={{
                            marginTop: space[3],
                            fontSize: 12,
                            color: palette.textSecondary,
                        }}
                    >
                        Live metrics unavailable — showing last-known values. ({sysInfoError})
                    </div>
                )}
            </Card>

            <Card header="Network" padding={5}>
                <DataList
                    items={[
                        { label: "primary IP", value: primaryIP ? <Mono>{primaryIP}</Mono> : "—" },
                        { label: "primary MAC", value: primaryMAC ? <Mono>{primaryMAC}</Mono> : "—" },
                        {
                            label: "default gateway",
                            value: sysInfo?.default_gateway ? <Mono>{sysInfo.default_gateway}</Mono> : "—",
                        },
                        {
                            label: "public IP",
                            value: sysInfo?.public_ip ? <Mono>{sysInfo.public_ip}</Mono> : "—",
                        },
                    ]}
                />
                {sysInfo?.interfaces && sysInfo.interfaces.length > 0 && (
                    <div style={{ marginTop: space[4] }}>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead className="w-[160px]">interface</TableHead>
                                    <TableHead>MAC</TableHead>
                                    <TableHead>addresses</TableHead>
                                    <TableHead className="w-[100px]">state</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {sysInfo.interfaces.map((ifi) => (
                                    <TableRow key={ifi.name}>
                                        <TableCell>
                                            <Mono>{ifi.name}</Mono>
                                        </TableCell>
                                        <TableCell>
                                            {ifi.mac ? <Mono size={11}>{ifi.mac}</Mono> : "—"}
                                        </TableCell>
                                        <TableCell>
                                            {ifi.addrs && ifi.addrs.length > 0 ? (
                                                <Mono size={11}>{ifi.addrs.join(", ")}</Mono>
                                            ) : (
                                                "—"
                                            )}
                                        </TableCell>
                                        <TableCell>
                                            {ifi.is_up ? (
                                                <StatusPill tone="success">up</StatusPill>
                                            ) : (
                                                <StatusPill tone="neutral">down</StatusPill>
                                            )}
                                        </TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    </div>
                )}
            </Card>

            {sysInfo?.disks && sysInfo.disks.length > 0 && (
                <Card header="Storage" padding={5}>
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>mount</TableHead>
                                <TableHead>device</TableHead>
                                <TableHead className="w-[90px]">fs</TableHead>
                                <TableHead className="w-[120px]">used</TableHead>
                                <TableHead className="w-[120px]">total</TableHead>
                                <TableHead className="w-[70px]">%</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {sysInfo.disks.map((d, i) => (
                                <TableRow key={`${d.mountpoint}-${i}`}>
                                    <TableCell>
                                        <Mono>{d.mountpoint}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        <Mono size={11}>{d.device || "—"}</Mono>
                                    </TableCell>
                                    <TableCell>{d.fstype || "—"}</TableCell>
                                    <TableCell>{formatBytes(d.used_bytes)}</TableCell>
                                    <TableCell>{formatBytes(d.total_bytes)}</TableCell>
                                    <TableCell>{formatPercent(d.used_bytes, d.total_bytes)}</TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </Card>
            )}

            {sysInfo?.users && sysInfo.users.length > 0 && (
                <Card header="Logged-in users" padding={5}>
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[140px]">user</TableHead>
                                <TableHead className="w-[140px]">terminal</TableHead>
                                <TableHead>from</TableHead>
                                <TableHead className="w-[160px]">since</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {sysInfo.users.map((u, i) => (
                                <TableRow key={`${u.user}-${u.terminal}-${i}`}>
                                    <TableCell>
                                        <Mono>{u.user || "—"}</Mono>
                                    </TableCell>
                                    <TableCell>
                                        <Mono size={11}>{u.terminal || "—"}</Mono>
                                    </TableCell>
                                    <TableCell>{u.host ? <Mono size={11}>{u.host}</Mono> : "—"}</TableCell>
                                    <TableCell className="text-text-secondary">
                                        {u.started_at
                                            ? fromNow(new Date(u.started_at * 1000).toISOString())
                                            : "—"}
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </Card>
            )}
        </div>
    );
}

// formatBytes turns a byte count into a short human-friendly label
// (e.g. "124 GB"). Returns "—" for undefined / zero so tables align.
function formatBytes(n?: number): string {
    if (!n || n <= 0) return "—";
    const units = ["B", "KB", "MB", "GB", "TB", "PB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

function formatPercent(used?: number, total?: number): string {
    if (!used || !total || total <= 0) return "—";
    return `${((used / total) * 100).toFixed(1)} %`;
}

function renderMemoryLine(used?: number, total?: number, available?: number): React.ReactNode {
    if (!total) return "—";
    const pct = used ? ` · ${((used / total) * 100).toFixed(1)} %` : "";
    return (
        <span>
            {formatBytes(used)} / {formatBytes(total)}
            {pct}
            {available ? ` · ${formatBytes(available)} avail` : ""}
        </span>
    );
}

function renderLoadLine(l1?: number, l5?: number, l15?: number): React.ReactNode {
    if (l1 === undefined && l5 === undefined && l15 === undefined) return "—";
    const fmt = (n?: number) => (n === undefined ? "—" : n.toFixed(2));
    return (
        <Mono>
            {fmt(l1)} · {fmt(l5)} · {fmt(l15)}
        </Mono>
    );
}

function renderUptime(secs?: number, bootUnix?: number): React.ReactNode {
    if (!secs && !bootUnix) return "—";
    const s = secs ?? (bootUnix ? Math.max(0, Math.floor(Date.now() / 1000) - bootUnix) : 0);
    if (!s) return "—";
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    const parts: string[] = [];
    if (d) parts.push(`${d}d`);
    if (h || d) parts.push(`${h}h`);
    parts.push(`${m}m`);
    return parts.join(" ");
}

function SessionsPanel({ sessions }: { sessions: SessionRow[] }) {
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
                        <TableHead>Ingress</TableHead>
                        <TableHead>User</TableHead>
                        <TableHead>Remote</TableHead>
                        <TableHead className="w-[140px]">Connected</TableHead>
                        <TableHead className="w-[180px]">Status</TableHead>
                    </TableRow>
                </TableHeader>
                <TableBody>
                    {sessions.map((r) => {
                        return (
                            <TableRow key={r.id}>
                                <TableCell>
                                    <Mono>{`${r.id.slice(0, 16)}…`}</Mono>
                                </TableCell>
                                <TableCell>
                                    {r.ingress_addr ? <Mono>{r.ingress_addr}</Mono> : "—"}
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
