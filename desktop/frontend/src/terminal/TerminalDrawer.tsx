import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { ChevronDown, ChevronUp, Plus, TerminalSquare, X } from "lucide-react";

import Terminal from "../pages/Terminal";
import { palette, radius, space } from "../layout/theme";
import { Host, listHosts } from "../lib/api";
import { useShell } from "../layout/ProjectShell";
import { colorForId } from "../lib/colors";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ShellEntry, useGlobalTerminal } from "./GlobalTerminalContext";

// Tab bar height constant is exported because ShellChrome needs to size
// the resizable drawer panel to exactly the tab bar when the operator
// "collapses" the drawer (Ctrl+`) — the body slab hides but the tabs
// stay visible so they can click an existing tab to expand again.
export const TAB_BAR_HEIGHT = 36;

// TerminalDrawer is the VSCode-style bottom panel mounted once inside
// ShellChrome's content column — it sits beneath <main> and beside
// (not under) ProjectSidebar, so it spans only the project content
// area.
//
// Persistence model: as long as any shell exists in the global
// terminal context, the drawer container stays in the React tree so
// every <Terminal> can stay mounted. That means xterm + the
// underlying WebSocket survive route changes (Overview ↔ host page)
// AND host switches — without that, navigating away would dispose the
// WebSocket and kick whatever was attached on the agent (e.g. a tmux
// client). Visibility is layered on top:
//   · on a non-host route, OR a host page with no shells of its own,
//     the drawer collapses to height 0 and visibility:hidden so the
//     bottom of the screen is free for the page itself.
//   · on a host page with shells, the drawer reveals normally and
//     the tab bar lists only that host's shells (cross-host
//     visibility lives in the status-bar terminals popover).
//   · when the operator collapses the drawer (Ctrl+`), only the tab
//     strip is shown; xterm content slab is display:none but still
//     mounted so scrollback survives.
export default function TerminalDrawer() {
    const {
        shells,
        activeId,
        drawerOpen,
        closeShell,
        setActive,
        openDrawer,
        closeDrawer,
    } = useGlobalTerminal();

    const { hostId: routeHostId } = useParams<{
        projectSlug?: string;
        hostId?: string;
        tab?: string;
    }>();

    const visibleShells = useMemo(
        () => shells.filter((s) => s.hostId === routeHostId),
        [shells, routeHostId],
    );

    // If the active shell isn't on this host (e.g. user just
    // navigated in), fall back to the first visible shell so the
    // tab bar always has something selected.
    const visibleActiveId = useMemo(() => {
        if (visibleShells.some((s) => s.id === activeId)) return activeId;
        return visibleShells.length > 0 ? visibleShells[0].id : null;
    }, [visibleShells, activeId]);

    // Nothing to keep alive: no terminals anywhere. The parent
    // PanelGroup's drawer panel is sized to 0 in this case so no
    // empty chrome leaks through.
    if (shells.length === 0) {
        return null;
    }

    // Drawer is "active" — i.e. visually present — only on a host
    // detail page that has at least one shell on that host. When
    // inactive, we still render the container (so <Terminal>
    // children stay mounted) but the parent panel collapses us to
    // zero height; visibility:hidden keeps the unrendered chrome
    // from peeking through during the resize transition.
    const drawerActive = !!routeHostId && visibleShells.length > 0;

    // Host indicator colour — every tab in this drawer belongs to
    // the same host (we filtered by routeHostId), so a single
    // accent stripe along the top of the drawer reads as "this is
    // host X" without needing a per-tab dot.
    const hostAccent = routeHostId ? colorForId(routeHostId) : palette.border;

    return (
        <div
            data-testid="terminal-drawer"
            data-active={drawerActive ? "true" : "false"}
            data-collapsed={drawerOpen ? "false" : "true"}
            data-host-id={routeHostId ?? ""}
            style={{
                position: "relative",
                height: "100%",
                width: "100%",
                borderTop: drawerActive ? `2px solid ${hostAccent}` : "none",
                background: palette.main,
                display: "flex",
                flexDirection: "column",
                visibility: drawerActive ? "visible" : "hidden",
                overflow: "hidden",
            }}
        >
            {drawerActive && (
                <TabBar
                    shells={visibleShells}
                    activeId={visibleActiveId}
                    onActivate={setActive}
                    onClose={closeShell}
                    onToggleDrawer={drawerOpen ? closeDrawer : openDrawer}
                    drawerOpen={drawerOpen}
                />
            )}
            <div
                style={{
                    flex: 1,
                    minHeight: 0,
                    position: "relative",
                    display: drawerActive && drawerOpen ? "block" : "none",
                }}
            >
                {/* Render <Terminal> for *every* shell, not just the
                    ones for the current host. Otherwise navigating
                    off the host page (or to a different host) would
                    unmount the xterm + WebSocket, which kicks the
                    operator's tmux client on the agent. Off-host
                    and non-active shells are display:none but still
                    fully mounted; xterm continues to buffer
                    incoming output and the WebSocket stays open. */}
                {shells.map((s) => {
                    const onCurrentHost = s.hostId === routeHostId;
                    const visible =
                        drawerActive && onCurrentHost && s.id === visibleActiveId;
                    return (
                        <div
                            key={s.id}
                            style={{
                                position: "absolute",
                                inset: 0,
                                display: visible ? "block" : "none",
                            }}
                        >
                            <Terminal
                                shellId={s.id}
                                projectID={s.projectID}
                                sessionHash={s.sessionHash}
                                onClose={() => closeShell(s.id)}
                            />
                        </div>
                    );
                })}
            </div>
        </div>
    );
}

interface TabBarProps {
    shells: ShellEntry[];
    activeId: string | null;
    onActivate: (id: string) => void;
    onClose: (id: string) => void;
    onToggleDrawer: () => void;
    drawerOpen: boolean;
}

function TabBar({
    shells,
    activeId,
    onActivate,
    onClose,
    onToggleDrawer,
    drawerOpen,
}: TabBarProps) {
    const navigate = useNavigate();
    const activeShell = shells.find((s) => s.id === activeId) ?? null;

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                height: TAB_BAR_HEIGHT,
                minHeight: TAB_BAR_HEIGHT,
                borderBottom: `1px solid ${palette.border}`,
                paddingLeft: space[2],
                paddingRight: space[2],
                gap: space[1],
                background: palette.sidebar,
            }}
        >
            <TerminalSquare
                className="size-3.5"
                style={{ color: palette.textMuted, marginRight: space[1] }}
            />
            <div
                style={{
                    flex: 1,
                    display: "flex",
                    alignItems: "center",
                    gap: 2,
                    overflowX: "auto",
                    overflowY: "hidden",
                    scrollbarWidth: "thin",
                }}
            >
                {shells.map((s) => {
                    const active = s.id === activeId;
                    return (
                        <div
                            key={s.id}
                            role="tab"
                            aria-selected={active}
                            onClick={() => onActivate(s.id)}
                            onDoubleClick={() =>
                                navigate(`/projects/${s.projectSlug}/fleet/hosts/${s.hostId}/files`)
                            }
                            title={`Double-click to jump to host`}
                            style={{
                                display: "inline-flex",
                                alignItems: "center",
                                gap: space[2],
                                padding: `4px ${space[2]}px 4px ${space[3]}px`,
                                background: active ? palette.surface : "transparent",
                                border: `1px solid ${
                                    active ? palette.border : "transparent"
                                }`,
                                borderBottom: active
                                    ? `1px solid ${palette.surface}`
                                    : `1px solid transparent`,
                                borderRadius: `${radius.sm}px ${radius.sm}px 0 0`,
                                color: active ? palette.textPrimary : palette.textSecondary,
                                fontSize: 12,
                                fontFamily: "var(--font-geist-mono)",
                                cursor: "pointer",
                                userSelect: "none",
                                transition: "color 120ms ease, background 120ms ease",
                                whiteSpace: "nowrap",
                            }}
                        >
                            <span>{s.label}</span>
                            <button
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onClose(s.id);
                                }}
                                style={{
                                    display: "inline-flex",
                                    alignItems: "center",
                                    justifyContent: "center",
                                    width: 16,
                                    height: 16,
                                    padding: 0,
                                    background: "none",
                                    border: "none",
                                    color: palette.textMuted,
                                    cursor: "pointer",
                                }}
                                aria-label="Close shell"
                                title="Close shell"
                            >
                                <X className="size-3" />
                            </button>
                        </div>
                    );
                })}
            </div>
            <NewShellButton activeShell={activeShell} />
            <button
                onClick={onToggleDrawer}
                aria-label={drawerOpen ? "Hide terminal panel" : "Show terminal panel"}
                title={
                    drawerOpen
                        ? "Hide panel (Ctrl+`)"
                        : "Show panel (Ctrl+`)"
                }
                data-testid="terminal-toggle-drawer"
                style={{
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    width: 24,
                    height: 24,
                    background: "none",
                    border: "none",
                    color: palette.textMuted,
                    cursor: "pointer",
                    borderRadius: radius.sm,
                }}
            >
                {drawerOpen ? (
                    <ChevronDown className="size-4" />
                ) : (
                    <ChevronUp className="size-4" />
                )}
            </button>
        </div>
    );
}

interface NewShellButtonProps {
    activeShell: {
        hostId: string;
        sessionHash: string;
        projectID: string;
        projectSlug: string;
        label: string;
    } | null;
}

// NewShellButton offers two affordances:
//   · click → opens a new shell on the host that owns the currently
//     active shell (the most common "another tab on the same host"
//     case, matching iTerm / VSCode).
//   · chevron → opens a dropdown listing live hosts in the current
//     project so an operator can open a shell on a different host
//     without leaving the drawer.
function NewShellButton({ activeShell }: NewShellButtonProps) {
    const { openShell } = useGlobalTerminal();
    const { project } = useShell();
    const navigate = useNavigate();
    const [menuOpen, setMenuOpen] = useState(false);
    const [hosts, setHosts] = useState<Host[] | null>(null);
    const [loading, setLoading] = useState(false);

    const quickNew = () => {
        if (!activeShell) return;
        openShell({
            projectID: activeShell.projectID,
            projectSlug: activeShell.projectSlug,
            hostId: activeShell.hostId,
            sessionHash: activeShell.sessionHash,
            // Keep the base label (strip any existing " · N" suffix) so
            // the registry's counter appends the correct index.
            label: activeShell.label.replace(/ · \d+$/, ""),
        });
    };

    useEffect(() => {
        if (!menuOpen || !project) return;
        let cancelled = false;
        setLoading(true);
        listHosts(project.id)
            .then((hs) => {
                if (!cancelled) setHosts(hs);
            })
            .catch(() => {
                if (!cancelled) setHosts([]);
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [menuOpen, project]);

    return (
        <div style={{ display: "inline-flex", alignItems: "center" }}>
            <button
                onClick={quickNew}
                disabled={!activeShell}
                aria-label="New shell on current host"
                title="New shell on current host"
                style={{
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                    width: 24,
                    height: 24,
                    background: "none",
                    border: "none",
                    color: activeShell ? palette.textSecondary : palette.textMuted,
                    cursor: activeShell ? "pointer" : "not-allowed",
                    borderRadius: radius.sm,
                    opacity: activeShell ? 1 : 0.5,
                }}
            >
                <Plus className="size-4" />
            </button>
            <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
                <DropdownMenuTrigger asChild>
                    <button
                        disabled={!project}
                        aria-label="Open shell on another host"
                        title="Open shell on another host"
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            justifyContent: "center",
                            width: 18,
                            height: 24,
                            background: "none",
                            border: "none",
                            color: project ? palette.textSecondary : palette.textMuted,
                            cursor: project ? "pointer" : "not-allowed",
                            borderRadius: radius.sm,
                        }}
                    >
                        <ChevronDown className="size-3" />
                    </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" side="top" className="min-w-[220px]">
                    <DropdownMenuLabel>Open shell on…</DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    {loading && (
                        <DropdownMenuItem disabled>Loading hosts…</DropdownMenuItem>
                    )}
                    {!loading && hosts && hosts.length === 0 && (
                        <DropdownMenuItem disabled>No hosts in project</DropdownMenuItem>
                    )}
                    {!loading &&
                        hosts &&
                        hosts
                            .filter((h) => !!h.agent_id)
                            .map((h) => {
                                const label =
                                    h.primary_alias ||
                                    h.hostname ||
                                    h.machine_id?.slice(0, 8) ||
                                    "unknown";
                                return (
                                    <DropdownMenuItem
                                        key={h.id}
                                        onSelect={() => {
                                            if (!project || !h.agent_id) return;
                                            openShell({
                                                projectID: project.id,
                                                projectSlug: project.slug,
                                                hostId: h.id,
                                                sessionHash: h.agent_id,
                                                label,
                                            });
                                            // The drawer is now host-scoped — the
                                            // newly-opened shell only becomes
                                            // visible when the operator is on
                                            // that host's detail page, so
                                            // navigate there instead of just
                                            // dropping a shell into a hidden
                                            // bucket.
                                            navigate(
                                                `/projects/${project.slug}/fleet/hosts/${h.id}/files`,
                                            );
                                        }}
                                    >
                                        {label}
                                    </DropdownMenuItem>
                                );
                            })}
                </DropdownMenuContent>
            </DropdownMenu>
        </div>
    );
}
