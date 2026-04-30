import {
    Link,
    Outlet,
    useLocation,
    useNavigate,
    useParams,
} from "react-router-dom";
import { ShieldAlert } from "lucide-react";
import { useCallback, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";

import EnrollmentWaitBanner from "../components/EnrollmentWaitBanner";
import PageShell from "../components/PageShell";
import StatusPills from "../components/StatusPills";
import { useCurrentProject } from "../layout/ProjectShell";
import { cn } from "@/lib/cn";
import { icons } from "../lib/icons";
import { listHosts, pendingApprovalCount } from "../lib/api";
import { qk } from "../lib/queryKeys";
import { isOnline } from "../lib/time";
import { useGlobalTerminal } from "../terminal/GlobalTerminalContext";
import TerminalDrawer, { TAB_BAR_HEIGHT } from "../terminal/TerminalDrawer";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

import EnrollAgentWizard from "./fleet/enroll/EnrollAgentWizard";

// HostsShell is the parent route for /projects/:slug/hosts/*. Owns
// the page title (Hosts), online/offline pills, Enroll button, and
// the View toggle (List ↔ Topology). The host-detail master view
// lives under /hosts/:id/:tab and renders inside the same shell so
// the chrome stays in place when jumping between hosts.
//
// HostsShell also owns the bottom terminal drawer because every shell
// is host-scoped — the drawer should not appear on /activity, /security,
// /enrollment, etc. The drawer is mounted whenever any /hosts/* route
// is active and only renders content (height > 0) when the URL has a
// :hostId in scope. The GlobalTerminalProvider stays at ProjectShell
// level so existing shell sessions survive cross-tab navigation.
const VIEWS = ["list", "topology"] as const;
type HostsView = (typeof VIEWS)[number];

const VIEW_LABELS: Record<HostsView, string> = {
    list: "List",
    topology: "Topology",
};

export default function HostsShell() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const { hostId } = useParams<{ hostId?: string }>();

    // Pick the active view from the URL. /hosts → list, /hosts/topology
    // → topology, /hosts/:hostId/* → list (so the rail stays grouped
    // with the list-mode chrome).
    const segments = pathname.split("/").filter(Boolean);
    const hostsIdx = segments.indexOf("hosts");
    const after = hostsIdx >= 0 ? segments[hostsIdx + 1] : undefined;
    const activeView: HostsView = after === "topology" ? "topology" : "list";

    const { data: hosts } = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });

    // Pending approvals badge polls every 10s. The badge surfaces in
    // the header so the operator sees pending approvals from anywhere
    // under /hosts; clicking jumps to /enrollment/approvals.
    const { data: pendingCount = 0 } = useQuery({
        queryKey: qk.pendingHostsCount(project.id),
        queryFn: () => pendingApprovalCount(project.id),
        refetchInterval: 10_000,
    });

    const counts = useMemo(() => {
        const list = hosts ?? [];
        let online = 0;
        for (const h of list) {
            if (isOnline(h.last_seen_at)) online++;
        }
        return { online, offline: list.length - online };
    }, [hosts]);

    const EnrollIcon = icons.enrollment;
    const actions = (
        <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
            {pendingCount > 0 && (
                <Button asChild variant="outline" size="sm" data-testid="hosts-pending-approvals">
                    <Link
                        to={`/projects/${project.slug}/enrollment/approvals`}
                        title="Hosts awaiting admin approval — agents can't open links until approved"
                    >
                        <ShieldAlert className="size-3.5" />
                        {pendingCount} pending
                    </Link>
                </Button>
            )}
            <Button asChild variant="outline" size="sm">
                <Link to="?enroll=1" data-testid="hosts-enroll-trigger">
                    <EnrollIcon className="size-3.5" />
                    Enroll agent
                </Link>
            </Button>
        </span>
    );

    // The List/Topology toggle hides itself when a specific host is
    // selected — the host detail view fills the surface and the
    // toggle would be visually unrelated to the master-detail rail.
    const tabs = !hostId ? (
        <Tabs
            value={activeView}
            onValueChange={(v) =>
                navigate(
                    v === "list"
                        ? `/projects/${project.slug}/hosts`
                        : `/projects/${project.slug}/hosts/${v}`,
                )
            }
        >
            <TabsList className="h-7" data-testid="hosts-subtabs">
                {VIEWS.map((v) => (
                    <TabsTrigger key={v} value={v}>
                        {VIEW_LABELS[v]}
                    </TabsTrigger>
                ))}
            </TabsList>
        </Tabs>
    ) : null;

    return (
        <PageShell
            title="Hosts"
            actions={actions}
            tabs={tabs}
            pills={
                <StatusPills
                    pills={[
                        { tone: "success", count: counts.online, label: "online" },
                        { tone: "muted", count: counts.offline, label: "offline" },
                    ]}
                />
            }
            bodyPadding={0}
            bodyStyle={{ display: "flex", flexDirection: "column", padding: 0 }}
        >
            <EnrollmentWaitBanner projectID={project.id} projectSlug={project.slug} />
            {/* Mounted at the parent so the wizard floats over any
                sub-view. Open / closed state is driven by the
                `?enroll=1` URL param via useEnrollWizardOpen. */}
            <EnrollAgentWizard />
            <HostsBody />
        </PageShell>
    );
}

// HostsBody stacks the routed view on top of the terminal drawer.
// The drawer has three regimes:
//   · no shells visible on this host  → 0 px (drawer hidden)
//   · drawer collapsed (Ctrl+`)        → TAB_BAR_HEIGHT (tab strip only)
//   · drawer open                      → drawerHeight (operator-chosen)
// drawerHeight is owned by GlobalTerminalContext (per-server
// localStorage); the seam's pointermove handler feeds drag deltas
// straight back into setDrawerHeight.
function HostsBody() {
    const { shells, drawerOpen, drawerHeight, setDrawerHeight } = useGlobalTerminal();
    const { hostId: routeHostId } = useParams<{ hostId?: string }>();
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
                <Outlet />
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
            {/* TerminalDrawer stays mounted across all regimes — the
                xterm WebSocket is owned by its children and would tear
                down on unmount. We just clamp height: 0 when inactive,
                TAB_BAR_HEIGHT when collapsed, drawerHeight when open. */}
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
