import { useSearchParams } from "react-router-dom";
import { Network, Rows3, Timer } from "lucide-react";

import EnrollmentWaitBanner from "../components/EnrollmentWaitBanner";
import PageHeader from "../components/PageHeader";
import { useCurrentProject } from "../layout/ProjectShell";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import HostsPanel from "./fleet/HostsPanel";
import SessionsPanel from "./fleet/SessionsPanel";
import TopologyPanel from "./fleet/TopologyPanel";

type FleetView = "table" | "timeline" | "graph";

const VIEWS: readonly FleetView[] = ["table", "timeline", "graph"] as const;

function parseView(raw: string | null): FleetView {
    return (VIEWS as readonly string[]).includes(raw ?? "")
        ? (raw as FleetView)
        : "table";
}

const SUBTITLES: Record<FleetView, string> = {
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
    const view = parseView(params.get("view"));

    const setView = (next: FleetView) => {
        const nextParams = new URLSearchParams(params);
        if (next === "table") {
            nextParams.delete("view");
        } else {
            nextParams.set("view", next);
        }
        setParams(nextParams, { replace: true });
    };

    const switcher = (
        <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={view}
            onValueChange={(v) => {
                if (v) setView(v as FleetView);
            }}
        >
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
    );

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader title="Fleet" subtitle={SUBTITLES[view]} actions={switcher} />
            <EnrollmentWaitBanner projectID={project.id} projectSlug={project.slug} />
            <div style={{ flex: 1, minHeight: 0, position: "relative" }}>
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
        </div>
    );
}
