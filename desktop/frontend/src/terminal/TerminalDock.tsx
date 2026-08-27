import { useCallback, useMemo, useRef } from "react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { useGlobalTerminal } from "./GlobalTerminalContext";
import TerminalDrawer, { TAB_BAR_HEIGHT } from "./TerminalDrawer";
import { useRouteHostId } from "./useRouteHostId";

// TerminalDock stacks the routed page on top of the terminal drawer and
// owns the drag seam between them.
//
// It lives in ShellChrome, above the router Outlet, because
// TerminalDrawer renders one <Terminal> per open shell and each of
// those owns a WebSocket. Anything that unmounts the drawer closes
// every one of them and kicks whatever was attached on the agent side
// — a tmux client, a tail -f, a half-typed command.
//
// This used to sit in HostsShell, on the reasoning that shells are
// host-scoped so the drawer has no business on /activity or /security.
// That is true of *visibility* and TerminalDrawer already enforces it
// on its own: it collapses to zero height whenever the URL has no
// :hostId. It is not true of *mounting* — HostsShell unmounts the
// moment you click Overview, so the drawer went with it and every
// session died. GlobalTerminalProvider sitting higher up did not help,
// because it holds the shell list, not the mounted terminals.
//
// Three regimes, unchanged from before the move:
//   · no shells on this host → 0 px, drawer hidden
//   · collapsed (Ctrl+`)     → TAB_BAR_HEIGHT, tab strip only
//   · open                   → drawerHeight, operator-chosen
//
// drawerHeight is owned by GlobalTerminalContext (persisted per server
// in localStorage); the seam's pointermove feeds drag deltas back into
// setDrawerHeight.
export default function TerminalDock({ children }: { children: ReactNode }) {
    const { shells, drawerOpen, drawerHeight, setDrawerHeight } = useGlobalTerminal();
    const routeHostId = useRouteHostId();

    const visibleShells = useMemo(
        () => shells.filter((s) => s.hostId === routeHostId),
        [shells, routeHostId],
    );
    const drawerActive = !!routeHostId && visibleShells.length > 0;
    const seamLive = drawerActive && drawerOpen;

    const containerRef = useRef<HTMLDivElement>(null);

    const onSeamPointerDown = useCallback(
        (event: React.PointerEvent<HTMLDivElement>) => {
            if (!seamLive) return;
            event.preventDefault();
            const seam = event.currentTarget;
            seam.setPointerCapture(event.pointerId);

            const onMove = (ev: PointerEvent) => {
                const container = containerRef.current;
                if (!container) return;
                const rect = container.getBoundingClientRect();
                setDrawerHeight(rect.bottom - ev.clientY);
            };
            const onUp = (ev: PointerEvent) => {
                if (seam.hasPointerCapture(ev.pointerId)) {
                    seam.releasePointerCapture(ev.pointerId);
                }
                window.removeEventListener("pointermove", onMove);
                window.removeEventListener("pointerup", onUp);
                window.removeEventListener("pointercancel", onUp);
            };
            window.addEventListener("pointermove", onMove);
            window.addEventListener("pointerup", onUp);
            window.addEventListener("pointercancel", onUp);
        },
        [seamLive, setDrawerHeight],
    );

    const drawerPx = !drawerActive ? 0 : drawerOpen ? drawerHeight : TAB_BAR_HEIGHT;

    return (
        <div
            ref={containerRef}
            style={{
                flex: 1,
                minHeight: 0,
                display: "flex",
                flexDirection: "column",
                position: "relative",
            }}
        >
            <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
                {children}
            </div>
            {/* Drag seam: visible-but-inert when the drawer is collapsed,
                interactive when open, hidden when no shells exist. */}
            <div
                role="separator"
                aria-orientation="horizontal"
                aria-disabled={!seamLive}
                onPointerDown={onSeamPointerDown}
                className={cn(
                    "relative h-px shrink-0 touch-none",
                    drawerActive ? "bg-border" : "invisible",
                    seamLive
                        ? "cursor-row-resize hover:bg-primary/40"
                        : "pointer-events-none",
                    "after:absolute after:inset-x-0 after:-inset-y-1 after:bg-transparent",
                )}
            />
            {/* Height is clamped here rather than inside the drawer so
                the <Terminal> children stay mounted at every regime. */}
            <div
                style={{
                    height: drawerPx,
                    flexShrink: 0,
                    overflow: "hidden",
                }}
            >
                <TerminalDrawer />
            </div>
        </div>
    );
}
