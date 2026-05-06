import {
    useCallback,
    useEffect,
    useLayoutEffect,
    useMemo,
    useRef,
    useState,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, TerminalSquare } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import EmptyState from "../components/EmptyState";
import RefreshButton from "../components/RefreshButton";
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
import { useGlobalTerminal } from "../terminal/GlobalTerminalContext";

import { decideAutoOpenShell } from "./host/autoOpenShell";
import { computeScrollSwap } from "./host/scrollPreservation";
import ActivityBar, { ACTIVITIES, Activity } from "./host/ActivityBar";
import {
    parsePluginActivity,
    visiblePluginEntries,
    type PluginUIEntry,
} from "./host/plugins/registry";
import BottomPanel, { BottomTab } from "./host/BottomPanel";
import FilesTab from "./host/FilesTab";
import HostHeaderBar from "./host/HostHeaderBar";
import InfoTab from "./host/InfoTab";
import ProcessesTab from "./host/ProcessesTab";
import RequiresPlugins from "./host/RequiresPlugins";

import { activitiesNeedingInstall, useInstalledPluginIDs } from "../lib/activityPlugins";
import SecurityTab from "./host/SecurityTab";
import ConfigTab from "./host/ConfigTab";
import SessionsTab from "./host/SessionsTab";
import TunnelsTab from "./host/TunnelsTab";
import PluginsTab from "./host/PluginsTab";

import { Button } from "@/components/ui/button";

interface Props {
    projectID: string;
    hostID: string;
}

// HostView is the right-pane VSCode-style layout for a selected host.
// Anatomy:
//   · HostHeaderBar — back link + identity pills + actions
//   · ActivityBar (44 px) — vertical icons for the 6 activities
//   · ActivityPane     — renders the active activity body
//   · BottomPanel (collapsible) — Processes / Tunnels for "peek
//     while editing"; defaults collapsed.
//
// URL-driven activity selection so deep links (`/hosts/<id>/files`)
// keep working — the slugs match the legacy tab keys (files / info /
// sessions / processes / security / tunnels) so existing bookmarks
// resolve.

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
    const agentID = host?.agent_id ?? "";
    const hostOS = host?.os ?? "";
    // Per-tab plugin gating. Reads the agent's installed-plugins list
    // once, drives both the dimmed-icon visual on ActivityBar and the
    // RequiresPlugins guard inside each tab body. Same query-key as
    // PluginsTab so an install there refreshes here without an
    // explicit refetch.
    const installedPlugins = useInstalledPluginIDs(projectID, agentID);
    const needsInstall = activitiesNeedingInstall(installedPlugins.ids);

    // Plugin-shipped activity entries from PLUGIN_UI_REGISTRY,
    // filtered by what's installed AND the host's runtime.GOOS.
    // Empty until the installed-plugins query resolves; the
    // ActivityBar handles `[]` cleanly (no divider, no extra icons).
    const pluginEntries: ReadonlyArray<PluginUIEntry> = useMemo(
        () => visiblePluginEntries(installedPlugins.ids ?? null, hostOS),
        [installedPlugins.ids, hostOS],
    );
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
    // Activity = either a hardcoded first-party slug OR a plugin
    // activity key of the form `plugin:<plugin_id>`. Validate against
    // both before defaulting to "files".
    const tabIsKnown =
        (ACTIVITIES as readonly string[]).includes(tabParam ?? "") ||
        (() => {
            const parsed = parsePluginActivity(tabParam ?? "");
            if (!parsed) return false;
            return pluginEntries.some((e) => e.pluginID === parsed.pluginID);
        })();
    const activeActivity: Activity = tabIsKnown
        ? (tabParam as Activity)
        : "files";
    const setActiveActivity = (key: Activity) =>
        navigate(`/projects/${project.slug}/hosts/${hostID}/${key}`);

    // BottomPanel state (open/collapsed, active tab, height) lives
    // here rather than inside BottomPanel because we want it sticky
    // across activity switches. localStorage persistence is intentionally
    // out-of-scope for v1 — operators who keep the panel open can re-
    // expand on next load (one click).
    const [bottomOpen, setBottomOpen] = useState(false);
    const [bottomTab, setBottomTab] = useState<BottomTab>("processes");
    const [bottomHeight, setBottomHeight] = useState(220);

    // Per-activity scroll preservation. Each activity panel shares one
    // scroll container; without help every switch resets scrollTop to
    // 0. computeScrollSwap is the pure brain.
    const scrollRef = useRef<HTMLDivElement | null>(null);
    const scrollMapRef = useRef(new Map<string, number>());
    const prevTabRef = useRef<string | null>(null);
    useLayoutEffect(() => {
        const el = scrollRef.current;
        if (!el) {
            prevTabRef.current = activeActivity;
            return;
        }
        const result = computeScrollSwap(
            scrollMapRef.current,
            prevTabRef.current,
            el.scrollTop,
            activeActivity,
        );
        scrollMapRef.current = result.map;
        el.scrollTop = result.scrollTop;
        prevTabRef.current = activeActivity;
    }, [activeActivity]);

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
        const next = live.length > 0 && host?.agent_id ? host.agent_id : null;
        if (pickedSessionID !== next) {
            setPickedSessionID(next);
        }
    }, [sessions, host?.agent_id, pickedSessionID]);

    // Auto-open a terminal the first time the operator lands on a
    // host that's reachable. The motivating UX: opening a host from
    // Fleet usually means "I need a shell here" — making the operator
    // click "Open terminal" again duplicates intent. Decision lives in
    // a pure helper so the contract stays unit-testable.
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
        if (!host?.agent_id) return;
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
    const liveSessions = sessions.filter((s) => !s.disconnected_at);
    const liveCount = liveSessions.length;
    const canOpenShell = liveCount > 0 && !!host.agent_id;

    const headerActions = (
        <span style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
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
                title={canOpenShell ? "Open a shell in the bottom drawer" : "No live agent session"}
            >
                <TerminalSquare className="size-3.5" />
            </Button>
            <RefreshButton
                loading={loading}
                onClick={refresh}
                iconOnly
                aria-label="Refresh"
                title="Refresh host"
            />
        </span>
    );

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 0 }}>
            <HostHeaderBar project={project} host={host} actions={headerActions} />
            <div style={{ flex: 1, minHeight: 0, display: "flex" }}>
                <ActivityBar
                    active={activeActivity}
                    onSelect={setActiveActivity}
                    badges={{ sessions: sessions.length || undefined }}
                    needsInstall={needsInstall}
                    pluginEntries={pluginEntries}
                />
                <div
                    style={{
                        flex: 1,
                        minWidth: 0,
                        minHeight: 0,
                        display: "flex",
                        flexDirection: "column",
                    }}
                >
                    <div
                        ref={scrollRef}
                        style={{
                            flex: 1,
                            minHeight: 0,
                            // Files manages its own internal scroll (file
                            // list and viewer scroll independently); the
                            // outer container must not race with the inner
                            // regions or the toggle/breadcrumb chrome
                            // would slide below the fold. Other activities
                            // are card stacks that need outer scroll.
                            overflow: activeActivity === "files" ? "hidden" : "auto",
                            display: "flex",
                            flexDirection: "column",
                        }}
                    >
                        {/* Each activity stays mounted (display:none on
                            the inactive ones) so expensive children (file
                            tree, processes poller, …) keep their state on
                            switch. */}
                        <div
                            style={{
                                display: activeActivity === "files" ? "flex" : "none",
                                flexDirection: "column",
                                flex: 1,
                                minHeight: 0,
                                padding: space[3],
                            }}
                        >
                            <RequiresPlugins
                                projectID={projectID}
                                agentID={agentID}
                                activity="files"
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
                            </RequiresPlugins>
                        </div>
                        <div
                            style={{
                                display: activeActivity === "info" ? "block" : "none",
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
                                display: activeActivity === "sessions" ? "block" : "none",
                                padding: space[4],
                            }}
                        >
                            <RequiresPlugins
                                projectID={projectID}
                                agentID={agentID}
                                activity="sessions"
                            >
                                <SessionsTab sessions={sessions} />
                            </RequiresPlugins>
                        </div>
                        <div
                            style={{
                                display: activeActivity === "processes" ? "block" : "none",
                                padding: space[4],
                            }}
                        >
                            <RequiresPlugins
                                projectID={projectID}
                                agentID={agentID}
                                activity="processes"
                            >
                                <ProcessesTab
                                    projectID={projectID}
                                    hostID={hostID}
                                    active={activeActivity === "processes"}
                                />
                            </RequiresPlugins>
                        </div>
                        <div
                            style={{
                                display: activeActivity === "security" ? "block" : "none",
                                padding: space[4],
                            }}
                        >
                            <RequiresPlugins
                                projectID={projectID}
                                agentID={agentID}
                                activity="security"
                            >
                                <SecurityTab
                                    projectID={projectID}
                                    hostID={hostID}
                                    active={activeActivity === "security"}
                                />
                            </RequiresPlugins>
                        </div>
                        <div
                            style={{
                                display: activeActivity === "config" ? "block" : "none",
                                padding: space[4],
                            }}
                        >
                            <RequiresPlugins
                                projectID={projectID}
                                agentID={agentID}
                                activity="config"
                            >
                                <ConfigTab
                                    projectID={projectID}
                                    hostID={hostID}
                                    active={activeActivity === "config"}
                                />
                            </RequiresPlugins>
                        </div>
                        <div
                            style={{
                                display: activeActivity === "tunnels" ? "block" : "none",
                                padding: space[4],
                                height: "100%",
                            }}
                        >
                            <RequiresPlugins
                                projectID={projectID}
                                agentID={agentID}
                                activity="tunnels"
                            >
                                <TunnelsTab projectID={projectID} hostID={hostID} />
                            </RequiresPlugins>
                        </div>
                        <div
                            style={{
                                display: activeActivity === "plugins" ? "block" : "none",
                                padding: space[4],
                            }}
                        >
                            <PluginsTab
                                projectID={projectID}
                                hostID={hostID}
                                agentID={agentID}
                                hostOS={hostOS}
                                active={activeActivity === "plugins"}
                            />
                        </div>
                        {/* Plugin-shipped activity bodies. Each
                            registry entry gets its own keyed-by-id
                            container so React preserves component
                            state when the operator switches between
                            sibling plugin tabs. */}
                        {pluginEntries.map((entry) => {
                            const isActive =
                                parsePluginActivity(activeActivity)?.pluginID ===
                                entry.pluginID;
                            const Component = entry.component;
                            return (
                                <div
                                    key={entry.pluginID}
                                    data-testid={`host-tab-body-${entry.pluginID}`}
                                    style={{
                                        display: isActive ? "block" : "none",
                                        padding: space[4],
                                    }}
                                >
                                    <Component
                                        projectID={projectID}
                                        agentID={agentID}
                                        hostOS={hostOS}
                                        active={isActive}
                                    />
                                </div>
                            );
                        })}
                    </div>
                    <BottomPanel
                        open={bottomOpen}
                        activeTab={bottomTab}
                        onActiveTabChange={setBottomTab}
                        onToggle={() => setBottomOpen((o) => !o)}
                        onClose={() => setBottomOpen(false)}
                        height={bottomHeight}
                        onHeightChange={setBottomHeight}
                    >
                        {bottomTab === "processes" && (
                            <div style={{ padding: space[3] }}>
                                <ProcessesTab
                                    projectID={projectID}
                                    hostID={hostID}
                                    active={bottomOpen && bottomTab === "processes"}
                                />
                            </div>
                        )}
                        {bottomTab === "tunnels" && (
                            <div style={{ padding: space[3], height: "100%" }}>
                                <TunnelsTab projectID={projectID} hostID={hostID} />
                            </div>
                        )}
                    </BottomPanel>
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
