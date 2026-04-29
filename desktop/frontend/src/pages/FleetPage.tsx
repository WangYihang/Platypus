import { Link, useSearchParams } from "react-router-dom";
import { LayoutGrid, Network, Rows3, Timer } from "lucide-react";
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import EnrollmentWaitBanner from "../components/EnrollmentWaitBanner";
import PageShell from "../components/PageShell";
import StatusPills from "../components/StatusPills";
import { useCurrentProject } from "../layout/ProjectShell";
import { icons } from "../lib/icons";
import { listHosts, pendingApprovalCount } from "../lib/api";
import { usePreference } from "../lib/preferences";
import { qk } from "../lib/queryKeys";
import { isOnline } from "../lib/time";
import { ShieldAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import HostsCardPanel from "./fleet/HostsCardPanel";
import HostsPanel from "./fleet/HostsPanel";
import SessionsPanel from "./fleet/SessionsPanel";
import TopologyPanel from "./fleet/TopologyPanel";
import EnrollAgentWizard from "./fleet/enroll/EnrollAgentWizard";

type FleetView = "cards" | "table" | "timeline" | "graph";

const VIEWS: readonly FleetView[] = ["cards", "table", "timeline", "graph"] as const;

function parseView(raw: string | null, fallback: FleetView): FleetView {
    return (VIEWS as readonly string[]).includes(raw ?? "")
        ? (raw as FleetView)
        : fallback;
}

const SUBTITLES: Record<FleetView, string> = {
    cards: "Hosts · card view",
    table: "Hosts · the inventory view",
    timeline: "Sessions · live and historical connections",
    graph: "Topology · mesh links between agents",
};

// FleetPage is the merged Hosts / Sessions / Topology surface. A single
// PageHeader with a Table / Timeline / Graph ToggleGroup switches
// between three children, all mounted with `display:none` for inactive
// panels so each view's local state (Cytoscape layout, search query,
// live/all filter) survives toggles.
export default function FleetPage() {
    const project = useCurrentProject();
    const [params, setParams] = useSearchParams();
    const [defaultView] = usePreference("ui.fleet.defaultView");
    const view = parseView(params.get("view"), defaultView);

    // The pages/fleet/Hosts*Panel children already query
    // `qk.hosts(project.id)` — we share that cache here so the
    // PageHeader pills don't double-fetch and stay in sync with the
    // table / cards body.
    const { data: hosts } = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });

    // Pending approvals badge: cheap COUNT(*) endpoint that polls every
    // 10s. Surfaced as a click-through pill in the StatusPills strip
    // when non-zero. Hidden when zero so the steady-state UI stays
    // uncluttered. The full list lives at /fleet/approvals.
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

    const setView = (next: FleetView) => {
        const nextParams = new URLSearchParams(params);
        if (next === defaultView) {
            nextParams.delete("view");
        } else {
            nextParams.set("view", next);
        }
        setParams(nextParams, { replace: true });
    };

    const switcher = (
        <span
            data-testid="fleet-view-toggle"
            title={`Default view: ${defaultView}. Change the default in Preferences (browser-local).`}
        >
            <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={view}
                onValueChange={(v) => {
                    if (v) setView(v as FleetView);
                }}
            >
                <ToggleGroupItem value="cards" aria-label="Card view">
                    <LayoutGrid className="size-3.5" />
                    Cards
                </ToggleGroupItem>
                <ToggleGroupItem value="table" aria-label="Table view">
                    <Rows3 className="size-3.5" />
                    Table
                </ToggleGroupItem>
                <ToggleGroupItem value="timeline" aria-label="Timeline view">
                    <Timer className="size-3.5" />
                    Timeline
                </ToggleGroupItem>
                <ToggleGroupItem value="graph" aria-label="Graph view">
                    <Network className="size-3.5" />
                    Graph
                </ToggleGroupItem>
            </ToggleGroup>
        </span>
    );

    // "Enroll agent" is the primary action on Fleet — it's how the
    // fleet grows. After the 2026-04 wizard refactor, the button no
    // longer routes to a separate page. It just sets `?enroll=1` on
    // the current URL, which the page-level <EnrollAgentWizard />
    // (mounted below) picks up to open. The same param is used by
    // the inline EnrollAgentTile inside HostsCardPanel and by the
    // Command Palette entry, so all three entry points share one
    // wire format and one open-state.
    const EnrollIcon = icons.enrollment;
    const actions = (
        <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
            {pendingCount > 0 && (
                <Button asChild variant="outline" size="sm" data-testid="fleet-pending-approvals">
                    <Link
                        to={`/projects/${project.slug}/fleet/approvals`}
                        title="Hosts awaiting admin approval — agents can't open links until approved"
                    >
                        <ShieldAlert className="size-3.5" />
                        {pendingCount} pending
                    </Link>
                </Button>
            )}
            <Button asChild variant="outline" size="sm">
                <Link to="?enroll=1" data-testid="fleet-enroll-trigger">
                    <EnrollIcon className="size-3.5" />
                    Enroll agent
                </Link>
            </Button>
            {switcher}
        </span>
    );

    return (
        <PageShell
            title="Fleet"
            subtitle={SUBTITLES[view]}
            actions={actions}
            pills={
                <StatusPills
                    pills={[
                        { tone: "success", count: counts.online, label: "online" },
                        { tone: "muted", count: counts.offline, label: "offline" },
                    ]}
                />
            }
            bodyPadding={0}
            bodyStyle={{ overflow: "visible", display: "flex", flexDirection: "column", padding: 0 }}
        >
            <EnrollmentWaitBanner projectID={project.id} projectSlug={project.slug} />
            {/* Mounted once at the page level so it floats over any
                of the four child views (cards / table / timeline /
                graph). Open / closed state is driven by the
                `?enroll=1` URL param (see useEnrollWizardOpen). */}
            <EnrollAgentWizard />
            <div style={{ flex: 1, minHeight: 0, position: "relative" }}>
                <div
                    data-testid="fleet-panel-cards"
                    aria-hidden={view !== "cards"}
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: view === "cards" ? "block" : "none",
                    }}
                >
                    <HostsCardPanel />
                </div>
                <div
                    data-testid="fleet-panel-table"
                    aria-hidden={view !== "table"}
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: view === "table" ? "block" : "none",
                    }}
                >
                    <HostsPanel />
                </div>
                <div
                    data-testid="fleet-panel-timeline"
                    aria-hidden={view !== "timeline"}
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: view === "timeline" ? "block" : "none",
                    }}
                >
                    <SessionsPanel />
                </div>
                <div
                    data-testid="fleet-panel-graph"
                    aria-hidden={view !== "graph"}
                    style={{
                        position: "absolute",
                        inset: 0,
                        display: view === "graph" ? "block" : "none",
                        overflow: "auto",
                    }}
                >
                    <TopologyPanel />
                </div>
            </div>
        </PageShell>
    );
}
