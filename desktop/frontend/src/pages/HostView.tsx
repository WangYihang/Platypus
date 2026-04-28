import * as React from "react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";

import { decideAutoOpenShell } from "./host/autoOpenShell";
import { computeScrollSwap } from "./host/scrollPreservation";
import {
    Boxes,
    HelpCircle,
    Laptop,
    Layers,
    Loader2,
    Monitor,
    Server,
    TerminalSquare,
} from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import Card from "../components/Card";
import DataList from "../components/DataList";
import EmptyState from "../components/EmptyState";
import MetricCard from "../components/MetricCard";
import Mono from "../components/Mono";
import RefreshButton from "../components/RefreshButton";
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
import { useGlobalTerminal } from "../terminal/GlobalTerminalContext";
import FilesTab from "./host/FilesTab";
import ProcessesTab from "./host/ProcessesTab";

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

const TABS = ["info", "files", "sessions", "processes"] as const;
type TabKey = (typeof TABS)[number];

// HostView is the main-panel view when a Host is selected. Four tabs
// (Info, Files, Sessions, Processes) live under the page header —
// shadcn Tabs for the bar, but the panels render ourselves so each
// tab stays mounted across switches. The shell surface moved out of
// this page into the global bottom drawer; operators open it via the
// "Open terminal" action in the header.
export default function HostView({ projectID, hostID }: Props) {
    const [host, setHost] = useState<Host | null>(null);
    const [sessions, setSessions] = useState<SessionRow[]>([]);
    const [sysInfo, setSysInfo] = useState<HostSysInfo | null>(null);
    const [sysInfoError, setSysInfoError] = useState<string | null>(null);
    const [sysInfoLoading, setSysInfoLoading] = useState(false);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    // pickedSessionID drives which session Terminal / Files operate
    // on. Despite the name, the value is the host's agent_id, not the
    // sessions-row UUID — every per-host RPC route on the server
    // (/api/v1/agents/:id/fs, /terminal/:id/ws, /rpc/:id, …) keys off
    // the agent_id from the cert SAN, which is what core.AgentLinkService
    // is registered under. Using the sessions-row id here would 404
    // because that's a fresh UUID per insert with no relationship to
    // the cert. The "session" framing stays in the variable name so
    // existing tab props keep working without churn.
    const [pickedSessionID, setPickedSessionID] = useState<string | null>(null);

    const project = useCurrentProject();
    const navigate = useNavigate();
    const { shells, openShell } = useGlobalTerminal();
    const { tab: tabParam } = useParams<{ tab?: string }>();
    const activeTab: TabKey = (TABS as readonly string[]).includes(tabParam ?? "")
        ? (tabParam as TabKey)
        : "info";
    const setActiveTab = (key: string) =>
        navigate(`/projects/${project.slug}/hosts/${hostID}/${key}`);

    // Per-tab scroll preservation. Each tab panel shares one scroll
    // container; without help every tab change resets scrollTop to
    // 0. computeScrollSwap is the pure brain — we read scrollTop off
    // the container before the tab swap, hand it the leaving tab,
    // and write back the restored value for the new tab.
    const scrollRef = useRef<HTMLDivElement | null>(null);
    const scrollMapRef = useRef(new Map<string, number>());
    const prevTabRef = useRef<string | null>(null);
    useLayoutEffect(() => {
        const el = scrollRef.current;
        if (!el) {
            prevTabRef.current = activeTab;
            return;
        }
        const result = computeScrollSwap(
            scrollMapRef.current,
            prevTabRef.current,
            el.scrollTop,
            activeTab,
        );
        scrollMapRef.current = result.map;
        el.scrollTop = result.scrollTop;
        prevTabRef.current = activeTab;
    }, [activeTab]);

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
        // No live session → blank the pick so tabs render empty state.
        // Any live session → pin pickedSessionID to host.agent_id (see
        // comment on the useState above). agent_id is single-valued
        // per host, so we don't need to disambiguate between concurrent
        // sessions on the same agent.
        const next = live.length > 0 && host?.agent_id ? host.agent_id : null;
        if (pickedSessionID !== next) {
            setPickedSessionID(next);
        }
    }, [sessions, host?.agent_id, pickedSessionID]);

    // Auto-open a terminal the first time the operator lands on a
    // host that's reachable. The motivating UX: opening a host from
    // Fleet usually means "I need a shell here" — making the operator
    // click "Open terminal" again duplicates intent. The decision
    // helper is pure (see ./host/autoOpenShell.ts) so the contract
    // is pinned by unit tests; this hook only handles the side
    // effects.
    const autoOpenedRef = useRef(false);
    useEffect(() => {
        const action = decideAutoOpenShell({
            alreadyAutoOpened: autoOpenedRef.current,
            hasAgentID: !!host?.agent_id,
            hasLiveSession: sessions.some((s) => !s.disconnected_at),
            shellAlreadyOpenForHost: shells.some((s) => s.hostId === hostID),
        });
        if (action.kind === "skip") return;
        autoOpenedRef.current = true;
        if (action.kind === "mark") return;
        if (!host?.agent_id) return; // narrowed by hasAgentID above; satisfies TS
        openShell({
            projectID: project.id,
            projectSlug: project.slug,
            hostId: hostID,
            sessionHash: host.agent_id,
            label: host.primary_alias || host.hostname || hostID.slice(0, 8),
        });
    }, [host, sessions, shells, hostID, project.id, project.slug, openShell]);

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
            <TabsList className="h-7">
                <TabsTrigger value="info">Info</TabsTrigger>
                <TabsTrigger value="files">Files</TabsTrigger>
                <TabsTrigger value="sessions">Sessions ({sessions.length})</TabsTrigger>
                <TabsTrigger value="processes">Processes</TabsTrigger>
            </TabsList>
        </Tabs>
    );

    const canOpenShell = liveCount > 0 && !!host.agent_id;
    // Icon-only buttons here so the page header doesn't grow with the
    // host alias — the tooltip + aria-label keep the action discoverable
    // without bloating the chrome.
    const openTerminalAction = (
        <Button
            size="icon-sm"
            variant="outline"
            disabled={!canOpenShell}
            onClick={() => {
                if (!host.agent_id) return;
                openShell({
                    projectID: project.id,
                    projectSlug: project.slug,
                    hostId: hostID,
                    sessionHash: host.agent_id,
                    label: primary,
                });
            }}
            aria-label="Open terminal"
            title={canOpenShell ? "Open a shell in the bottom panel" : "No live agent session"}
        >
            <TerminalSquare className="size-3.5" />
        </Button>
    );

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title={
                    <span style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
                        <StatusDot status={online ? "online" : "offline"} />
                        <span>{primary}</span>
                    </span>
                }
                subtitle={
                    <span>
                        {liveCount} active · {host.os || "unknown OS"}
                        {host.fingerprint_fallback && " · fp-fallback"}
                    </span>
                }
                actions={
                    <span style={{ display: "inline-flex", gap: space[2] }}>
                        {openTerminalAction}
                        <RefreshButton
                            loading={loading}
                            onClick={refresh}
                            iconOnly
                            aria-label="Refresh"
                            title="Refresh host"
                        />
                    </span>
                }
                tabs={tabBar}
            />
            <div
                ref={scrollRef}
                style={{
                    flex: 1,
                    minHeight: 0,
                    // Files tab manages its own internal scroll (file
                    // list and preview each scroll independently), so
                    // the outer container must not also scroll — that
                    // would race with the inner regions and trap the
                    // toggle/breadcrumb chrome below the fold. Other
                    // tabs are card stacks that need outer scroll.
                    overflow: activeTab === "files" ? "hidden" : "auto",
                    display: "flex",
                    flexDirection: "column",
                }}
            >
                {/* Each tab panel stays mounted (via display:none) so
                    expensive children (Files tree, Processes poller,
                    etc.) don't rebuild state on tab switch. */}
                <div
                    style={{
                        display: activeTab === "files" ? "flex" : "none",
                        flexDirection: "column",
                        flex: 1,
                        minHeight: 0,
                        padding: space[3],
                    }}
                >
                    {pickedSessionID ? (
                        <FilesTab
                            projectID={projectID}
                            sessionHash={pickedSessionID}
                            host={host}
                        />
                    ) : (
                        <NoLiveSessionNote />
                    )}
                </div>
                <div
                    style={{
                        display: activeTab === "sessions" ? "block" : "none",
                        padding: space[4],
                    }}
                >
                    <SessionsPanel sessions={sessions} />
                </div>
                <div
                    style={{
                        display: activeTab === "info" ? "block" : "none",
                        padding: space[4],
                    }}
                >
                    <InfoPanel
                        host={host}
                        sysInfo={sysInfo}
                        sysInfoError={sysInfoError}
                        sysInfoLoading={sysInfoLoading}
                        onRefreshSysInfo={refreshSysInfo}
                    />
                </div>
                <div
                    style={{
                        display: activeTab === "processes" ? "block" : "none",
                        padding: space[4],
                    }}
                >
                    <ProcessesTab
                        projectID={projectID}
                        hostID={hostID}
                        active={activeTab === "processes"}
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

// InfoPanel renders the host Info tab. Three regions, top-down:
//   1. KPI strip — five live metrics (CPU %, mem %, load1, disk %,
//      uptime) in a `auto-fit` MetricCard grid; cells hide when their
//      underlying field is missing so the strip degrades gracefully
//      when the agent is offline rather than rendering "—" placeholders.
//   2. Detail grid — Identity / System / Hardware / Network / Storage /
//      Logged-in users laid out in a two-column CSS grid (System spans
//      two rows because it carries the most data).
//   3. Inline cached-fallback note when the live sysInfo fetch failed.
// We prefer live SysInfo values when available and fall through to
// the DB-cached Host columns; that fallback is centralised in the
// individual cards.
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
        <div style={{ display: "flex", flexDirection: "column", gap: space[4], maxWidth: 1100 }}>
            <InfoKPIStrip host={host} sysInfo={sysInfo} />
            <div
                data-testid="host-info-detail-grid"
                style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fit, minmax(380px, 1fr))",
                    gap: space[3],
                    alignItems: "start",
                }}
            >
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
                        {
                            label: "machine type",
                            value: <MachineTypePill type={sysInfo?.machine_type || host.machine_type} />,
                        },
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
                        <RefreshButton
                            variant="ghost"
                            loading={sysInfoLoading}
                            onClick={onRefreshSysInfo}
                        />
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

            <HardwareCard host={host} sysInfo={sysInfo} />

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
                <Card
                    header="Logged-in users"
                    padding={5}
                    style={{ gridColumn: "1 / -1" }}
                >
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
        </div>
    );
}

// InfoKPIStrip renders the at-a-glance health row above the Info-tab
// detail grid. The five tiles cover the questions an operator asks
// in the first second after opening a host: is it pegged, out of
// memory, swapping, full, and how long has it been up? Cells render
// only when their underlying field is present so an offline agent
// produces a 0-tile strip rather than a row of "—" placeholders. The
// `auto-fit` grid wraps gracefully on narrow viewports (a 720 px
// sidebar+main layout collapses the 5 tiles to a 3+2 row pair).
function InfoKPIStrip({
    host,
    sysInfo,
}: {
    host: Host;
    sysInfo: HostSysInfo | null;
}) {
    interface Tile {
        key: string;
        label: string;
        value: ReactNode;
        hint?: ReactNode;
        accent?: "default" | "success" | "warning" | "danger";
    }
    const tiles: Tile[] = [];

    // CPU %. Warn at 70, danger at 90 — the thresholds match the
    // colours the operator already associates with the StatusPill
    // tones elsewhere on the page.
    if (sysInfo?.cpu_percent !== undefined) {
        const pct = sysInfo.cpu_percent;
        tiles.push({
            key: "cpu",
            label: "CPU",
            value: `${pct.toFixed(1)}%`,
            accent: pct >= 90 ? "danger" : pct >= 70 ? "warning" : "default",
        });
    }

    // Memory %. Computed from used/total — neither field alone is
    // useful as a KPI on its own (raw bytes scale with the host),
    // so we hide the tile unless both are present.
    const memTotal = sysInfo?.mem_total || host.mem_total_bytes;
    if (sysInfo?.mem_used !== undefined && memTotal) {
        const pct = (sysInfo.mem_used / memTotal) * 100;
        tiles.push({
            key: "mem",
            label: "Memory",
            value: `${pct.toFixed(1)}%`,
            hint: `${formatBytes(sysInfo.mem_used)} / ${formatBytes(memTotal)}`,
            accent: pct >= 90 ? "danger" : pct >= 70 ? "warning" : "default",
        });
    }

    // Load 1. Threshold is per-CPU because raw load1 means nothing
    // without core count — load1=4 on a 32-core box is idle, on a
    // 2-core box it's a full pegging.
    if (sysInfo?.load1 !== undefined) {
        const cores = sysInfo.num_cpu || host.num_cpu || 1;
        const ratio = sysInfo.load1 / cores;
        tiles.push({
            key: "load1",
            label: "Load 1m",
            value: sysInfo.load1.toFixed(2),
            hint:
                sysInfo.load5 !== undefined && sysInfo.load15 !== undefined
                    ? `${sysInfo.load5.toFixed(2)} · ${sysInfo.load15.toFixed(2)}`
                    : undefined,
            accent: ratio >= 2 ? "danger" : ratio >= 1 ? "warning" : "default",
        });
    }

    // Disk %. Pick the worst-utilised mount across all reported
    // disks. The hint names that mount so the operator can drill in
    // without having to scan the Storage table.
    if (sysInfo?.disks && sysInfo.disks.length > 0) {
        let worst: { pct: number; mount: string } | null = null;
        for (const d of sysInfo.disks) {
            if (!d.total_bytes || d.total_bytes <= 0) continue;
            const pct = ((d.used_bytes ?? 0) / d.total_bytes) * 100;
            if (!worst || pct > worst.pct) {
                worst = { pct, mount: d.mountpoint || "—" };
            }
        }
        if (worst) {
            tiles.push({
                key: "disk",
                label: "Disk",
                value: `${worst.pct.toFixed(1)}%`,
                hint: worst.mount,
                accent:
                    worst.pct >= 90 ? "danger" : worst.pct >= 80 ? "warning" : "default",
            });
        }
    }

    // Uptime. Reuses renderUptime() — already returns "Nd Nh Nm" or
    // "—". We only push the tile when the renderer would produce a
    // real string so the strip stays clean on never-seen hosts.
    const uptime = renderUptime(sysInfo?.uptime_seconds, sysInfo?.boot_time_unix || host.boot_time_unix);
    if (uptime !== "—") {
        tiles.push({ key: "uptime", label: "Uptime", value: uptime });
    }

    if (tiles.length === 0) return null;

    return (
        <div
            data-testid="host-info-kpi-strip"
            style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
                gap: space[3],
            }}
        >
            {tiles.map((t) => (
                <MetricCard
                    key={t.key}
                    label={t.label}
                    value={t.value}
                    hint={t.hint}
                    accent={t.accent}
                />
            ))}
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
                        <TableHead className="w-[120px]">Agent</TableHead>
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
                                <TableCell data-testid="session-version-cell">
                                    {r.version ? <Mono size={11}>{r.version}</Mono> : "—"}
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

// machineTypeMeta maps the coarse classification string to a label
// and a lucide icon. Keeping the mapping tight here (rather than in
// a shared util) so that adding a new category only touches one
// place per use site.
const machineTypeMeta: Record<
    string,
    { label: string; Icon: React.ComponentType<{ className?: string }> }
> = {
    container: { label: "container", Icon: Boxes },
    vm: { label: "virtual machine", Icon: Layers },
    bare_metal: { label: "bare metal", Icon: Server },
    laptop: { label: "laptop", Icon: Laptop },
    desktop: { label: "desktop", Icon: Monitor },
    unknown: { label: "unknown", Icon: HelpCircle },
};

function MachineTypePill({ type }: { type?: string }) {
    const meta = type ? machineTypeMeta[type] : undefined;
    if (!meta) return <>—</>;
    const { label, Icon } = meta;
    return (
        <span style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
            <Icon className="size-3.5" />
            <span>{label}</span>
        </span>
    );
}

// HardwareCard surfaces the chassis / product / BIOS identity plus
// the GPU list. Everything is optional — if the agent had no way to
// read DMI and ghw returned nothing we still render the card with
// "—" placeholders so operators can tell the probe ran but found
// nothing rather than "was this even collected?".
function HardwareCard({ host, sysInfo }: { host: Host; sysInfo: HostSysInfo | null }) {
    const productVendor = sysInfo?.product_vendor || host.product_vendor;
    const productName = sysInfo?.product_name || host.product_name;
    const biosVendor = sysInfo?.bios_vendor || host.bios_vendor;
    const biosVersion = sysInfo?.bios_version || host.bios_version;
    const chassis = sysInfo?.chassis_type || host.chassis_type;
    const containerRuntime = sysInfo?.container_runtime;

    const gpus = sysInfo?.gpus || [];

    return (
        <Card header="Hardware" padding={5}>
            <DataList
                items={[
                    {
                        label: "machine type",
                        value: (
                            <MachineTypePill type={sysInfo?.machine_type || host.machine_type} />
                        ),
                    },
                    ...(containerRuntime
                        ? [
                              {
                                  label: "container runtime",
                                  value: <Mono>{containerRuntime}</Mono>,
                              },
                          ]
                        : []),
                    {
                        label: "chassis",
                        value: chassis ? <Mono>{chassis}</Mono> : "—",
                    },
                    {
                        label: "product",
                        value: (
                            <span>
                                {productVendor || "—"}
                                {productName ? ` · ${productName}` : ""}
                            </span>
                        ),
                    },
                    {
                        label: "BIOS",
                        value: (
                            <span>
                                {biosVendor || "—"}
                                {biosVersion ? ` · ${biosVersion}` : ""}
                            </span>
                        ),
                    },
                ]}
            />
            {gpus.length > 0 && (
                <div style={{ marginTop: space[4] }}>
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[100px]">vendor</TableHead>
                                <TableHead>model</TableHead>
                                <TableHead className="w-[120px]">driver</TableHead>
                                <TableHead className="w-[120px]">VRAM</TableHead>
                                <TableHead className="w-[90px]">util</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {gpus.map((g, i) => (
                                <TableRow key={g.uuid || g.bus_id || `gpu-${i}`}>
                                    <TableCell>{g.vendor || "—"}</TableCell>
                                    <TableCell>{g.model || "—"}</TableCell>
                                    <TableCell>
                                        {g.driver ? (
                                            <Mono size={11}>
                                                {g.driver}
                                                {g.driver_version ? ` ${g.driver_version}` : ""}
                                            </Mono>
                                        ) : (
                                            "—"
                                        )}
                                    </TableCell>
                                    <TableCell>
                                        {g.vram_total_bytes
                                            ? `${formatBytes(g.vram_used_bytes)} / ${formatBytes(
                                                  g.vram_total_bytes,
                                              )}`
                                            : "—"}
                                    </TableCell>
                                    <TableCell>
                                        {g.utilization_pct !== undefined && g.utilization_pct > 0
                                            ? `${g.utilization_pct.toFixed(0)} %`
                                            : "—"}
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>
            )}
            {gpus.length === 0 && host.gpu_summary && (
                <div
                    style={{
                        marginTop: space[3],
                        fontSize: 12,
                        color: palette.textSecondary,
                    }}
                >
                    {host.gpu_summary}
                </div>
            )}
        </Card>
    );
}
