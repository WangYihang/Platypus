import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronDown, Plus, TerminalSquare, X } from "lucide-react";

import Terminal from "../pages/Terminal";
import { palette, radius, space } from "../layout/theme";
import { Host, listHosts } from "../lib/api";
import { useShell } from "../layout/ProjectShell";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ShellEntry, useGlobalTerminal } from "./GlobalTerminalContext";

const HANDLE_HEIGHT = 6;
const TAB_BAR_HEIGHT = 36;

// TerminalDrawer is the VSCode-style bottom panel mounted once inside
// ShellChrome, above StatusBar and outside <Routes>. Because the drawer
// lives at the app shell, the xterm instances it owns survive route
// changes — operators can switch between Fleet/Activities/Settings
// without losing scrollback or dropping their shell WebSocket.
export default function TerminalDrawer() {
    const {
        shells,
        activeId,
        drawerOpen,
        drawerHeight,
        closeShell,
        setActive,
        closeDrawer,
        setDrawerHeight,
    } = useGlobalTerminal();

    // When the drawer has nothing to show and isn't open we render
    // nothing at all so we don't reserve a row in the shell flexbox.
    if (!drawerOpen || shells.length === 0) {
        return null;
    }

    return (
        <div
            style={{
                position: "relative",
                height: drawerHeight,
                flexShrink: 0,
                borderTop: `1px solid ${palette.border}`,
                background: palette.main,
                display: "flex",
                flexDirection: "column",
            }}
        >
            <ResizeHandle height={drawerHeight} onResize={setDrawerHeight} />
            <TabBar
                shells={shells}
                activeId={activeId}
                onActivate={setActive}
                onClose={closeShell}
                onCloseDrawer={closeDrawer}
            />
            <div style={{ flex: 1, minHeight: 0, position: "relative" }}>
                {shells.map((s) => (
                    <div
                        key={s.id}
                        style={{
                            position: "absolute",
                            inset: 0,
                            display: s.id === activeId ? "block" : "none",
                        }}
                    >
                        <Terminal
                            sessionHash={s.sessionHash}
                            onClose={() => closeShell(s.id)}
                        />
                    </div>
                ))}
            </div>
        </div>
    );
}

interface ResizeHandleProps {
    height: number;
    onResize: (h: number) => void;
}

// ResizeHandle is a 6px-tall strip sitting on top of the drawer. It
// captures pointer events, switches the cursor to ns-resize, and
// forwards drag deltas as new drawer heights.
function ResizeHandle({ height, onResize }: ResizeHandleProps) {
    const startYRef = useRef<number | null>(null);
    const startHeightRef = useRef<number>(height);
    const [dragging, setDragging] = useState(false);

    const onPointerMove = useCallback(
        (e: PointerEvent) => {
            if (startYRef.current === null) return;
            const dy = startYRef.current - e.clientY;
            onResize(startHeightRef.current + dy);
        },
        [onResize],
    );

    const onPointerUp = useCallback(() => {
        startYRef.current = null;
        setDragging(false);
        window.removeEventListener("pointermove", onPointerMove);
        window.removeEventListener("pointerup", onPointerUp);
    }, [onPointerMove]);

    const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
        startYRef.current = e.clientY;
        startHeightRef.current = height;
        setDragging(true);
        window.addEventListener("pointermove", onPointerMove);
        window.addEventListener("pointerup", onPointerUp);
    };

    useEffect(() => {
        return () => {
            window.removeEventListener("pointermove", onPointerMove);
            window.removeEventListener("pointerup", onPointerUp);
        };
    }, [onPointerMove, onPointerUp]);

    return (
        <div
            onPointerDown={onPointerDown}
            style={{
                position: "absolute",
                top: -HANDLE_HEIGHT / 2,
                left: 0,
                right: 0,
                height: HANDLE_HEIGHT,
                cursor: "ns-resize",
                background: dragging ? palette.borderStrong : "transparent",
                zIndex: 2,
                transition: dragging ? "none" : "background 120ms ease",
            }}
            title="Drag to resize"
        />
    );
}

interface TabBarProps {
    shells: ShellEntry[];
    activeId: string | null;
    onActivate: (id: string) => void;
    onClose: (id: string) => void;
    onCloseDrawer: () => void;
}

function TabBar({
    shells,
    activeId,
    onActivate,
    onClose,
    onCloseDrawer,
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
                                navigate(`/projects/${s.projectSlug}/hosts/${s.hostId}/info`)
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
                                fontFamily: "var(--font-geist-sans)",
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
                onClick={onCloseDrawer}
                aria-label="Hide terminal panel"
                title="Hide panel (Ctrl+`)"
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
                <ChevronDown className="size-4" />
            </button>
        </div>
    );
}

interface NewShellButtonProps {
    activeShell: { hostId: string; sessionHash: string; projectSlug: string; label: string } | null;
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
    const [menuOpen, setMenuOpen] = useState(false);
    const [hosts, setHosts] = useState<Host[] | null>(null);
    const [loading, setLoading] = useState(false);

    const quickNew = () => {
        if (!activeShell) return;
        openShell({
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
                                                projectSlug: project.slug,
                                                hostId: h.id,
                                                sessionHash: h.agent_id,
                                                label,
                                            });
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
