import {
    useCallback,
    useEffect,
    useLayoutEffect,
    useRef,
    useState,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, TerminalSquare } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import EmptyState from "../components/EmptyState";
import PageHeader from "../components/PageHeader";
import RefreshButton from "../components/RefreshButton";
import StatusDot from "../components/StatusDot";
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
import { qk } from "../lib/queryKeys";
import { isOnline } from "../lib/time";
import { useGlobalTerminal } from "../terminal/GlobalTerminalContext";

import { decideAutoOpenShell } from "./host/autoOpenShell";
import { computeScrollSwap } from "./host/scrollPreservation";
import FilesTab from "./host/FilesTab";
import InfoTab from "./host/InfoTab";
import ProcessesTab from "./host/ProcessesTab";
import SessionsTab from "./host/SessionsTab";
import TunnelsTab from "./host/TunnelsTab";

import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface Props {
    projectID: string;
    hostID: string;
}

// HostView's tab order matches the VS Code mental model the R3
// redesign adopts: file browser is the centerpiece (this is a
// remote-shell client, the file system is what operators come here
// to explore), so Files leads. Info / Sessions / Processes / Tunnels
// follow as auxiliary panels. The route default
// (`/hosts/:id` → `/hosts/:id/files`) reflects that — landing on a
// system-info data dump was useful for debugging the agent but
// less useful for the day-to-day operator workflow.
const TABS = ["files", "info", "sessions", "processes", "tunnels"] as const;
type TabKey = (typeof TABS)[number];

// HostView is the main-panel view when a Host is selected. After the
// 2026-04 split, this file is just the tab orchestrator — each tab's
// rendering lives in pages/host/<Tab>.tsx so this surface stays
// focused on:
//
//   1. Fetching host / sysInfo / sessions and threading them down
//   2. Tab routing (URL-driven, deep-link friendly)
//   3. Per-tab scroll preservation
//   4. The auto-open-terminal heuristic
//   5. The page-level header chrome
export default function HostView({ projectID, hostID }: Props) {
    const queryClient = useQueryClient();
    const hostQuery = useQuery({
        queryKey: qk.host(projectID, hostID),
        queryFn: () => getHost(projectID, hostID),
    });
    const sessionsQuery = useQuery({
        queryKey: qk.hostSessions(projectID, hostID),
        queryFn: () => listHostSessions(projectID, hostID),
    });
    const sysInfoQuery = useQuery({
        queryKey: qk.hostSysInfo(projectID, hostID),
        queryFn: () => getHostSysInfo(projectID, hostID),
    });

    const host: Host | null = hostQuery.data ?? null;
    const sessions: SessionRow[] = sessionsQuery.data ?? [];
    const sysInfo: HostSysInfo | null = sysInfoQuery.data ?? null;
    const sysInfoError: string | null = sysInfoQuery.error
        ? String(sysInfoQuery.error)
        : null;
    const sysInfoLoading = sysInfoQuery.isFetching;
    const loading = hostQuery.isFetching || sessionsQuery.isFetching;
    const error: string | null = hostQuery.error
        ? String(hostQuery.error)
        : sessionsQuery.error
          ? String(sessionsQuery.error)
          : null;

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
    // container; without help every tab change resets scrollTop to 0.
    // computeScrollSwap is the pure brain — we read scrollTop off the
    // container before the tab swap, hand it the leaving tab, and
    // write back the restored value for the new tab.
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

    const refresh = useCallback(() => {
        queryClient.invalidateQueries({ queryKey: qk.host(projectID, hostID) });
        queryClient.invalidateQueries({ queryKey: qk.hostSessions(projectID, hostID) });
        queryClient.invalidateQueries({ queryKey: qk.hostSysInfo(projectID, hostID) });
    }, [queryClient, projectID, hostID]);

    const refreshSysInfo = useCallback(() => {
        queryClient.invalidateQueries({ queryKey: qk.hostSysInfo(projectID, hostID) });
    }, [queryClient, projectID, hostID]);

    const refetchSessions = useCallback(() => {
        queryClient.invalidateQueries({ queryKey: qk.hostSessions(projectID, hostID) });
    }, [queryClient, projectID, hostID]);

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
                <TabsTrigger value="files">Files</TabsTrigger>
                <TabsTrigger value="info">Info</TabsTrigger>
                <TabsTrigger value="sessions">Sessions ({sessions.length})</TabsTrigger>
                <TabsTrigger value="processes">Processes</TabsTrigger>
                <TabsTrigger value="tunnels">Tunnels</TabsTrigger>
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
                    <SessionsTab sessions={sessions} />
                </div>
                <div
                    style={{
                        display: activeTab === "info" ? "block" : "none",
                        padding: space[4],
                    }}
                >
                    <InfoTab
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
                <div
                    style={{
                        display: activeTab === "tunnels" ? "block" : "none",
                        padding: space[4],
                        height: "100%",
                    }}
                >
                    <TunnelsTab projectID={projectID} hostID={hostID} />
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
