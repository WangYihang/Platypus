// TopologyPage — the /projects/:slug/topology route.
//
// Composes the useTopology hook with the MeshGraph component and
// two selection-driven detail panels (MachineDetailPanel,
// LinkDetailPanel). The page itself is thin — all state lives in
// the hook so reconnects don't force a remount of the graph.

import { useMemo, useState } from "react";
import { Loader2 } from "lucide-react";

import Card from "../components/Card";
import PageHeader from "../components/PageHeader";
import { useCurrentProject } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";

import MeshGraph from "../components/topology/MeshGraph";
import MachineDetailPanel from "../components/topology/MachineDetailPanel";
import LinkDetailPanel from "../components/topology/LinkDetailPanel";
import { useTopology } from "./hooks/useTopology";

export default function TopologyPage() {
    const project = useCurrentProject();
    const state = useTopology(project.id);

    const [selectedMachine, setSelectedMachine] = useState<string | null>(null);
    const [selectedLink, setSelectedLink] = useState<string | null>(null);

    const machine = useMemo(
        () => (state.snapshot?.machines ?? []).find((m) => m.host_id === selectedMachine) ?? null,
        [state.snapshot, selectedMachine],
    );
    const link = useMemo(() => {
        if (!state.snapshot || !selectedLink) return null;
        const [a, b] = selectedLink.split("|");
        return (state.snapshot.links ?? []).find(
            (l) => (l.a === a && l.b === b) || (l.a === b && l.b === a),
        ) ?? null;
    }, [state.snapshot, selectedLink]);

    const headerMeta = state.snapshot
        ? `${(state.snapshot.machines ?? []).length} machine${(state.snapshot.machines ?? []).length === 1 ? "" : "s"} · ${(state.snapshot.links ?? []).length} link${(state.snapshot.links ?? []).length === 1 ? "" : "s"}${state.snapshot.mesh_enabled ? "" : " · hub-and-spoke"}`
        : undefined;

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4], height: "100%" }}>
            <PageHeader title="Topology" subtitle={headerMeta ?? "Loading topology…"} />

            {state.loading && !state.snapshot && (
                <Card>
                    <div style={{ display: "flex", alignItems: "center", gap: space[2], padding: space[6], color: palette.textMuted }}>
                        <Loader2 className="size-4 animate-spin" />
                        <span>Fetching mesh snapshot…</span>
                    </div>
                </Card>
            )}

            {state.error && (
                <Card>
                    <div style={{ padding: space[4], color: "#ef4444" }}>{state.error}</div>
                </Card>
            )}

            {state.snapshot && (
                <Card>
                    <div style={{ height: "calc(100vh - 220px)", minHeight: 400, position: "relative" }}>
                        {(state.snapshot.machines ?? []).length === 0 && (state.snapshot.mesh_nodes ?? []).length === 0 && (
                            <div
                                style={{
                                    position: "absolute",
                                    inset: 0,
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "center",
                                    color: palette.textMuted,
                                    fontSize: 13,
                                }}
                            >
                                No agents currently connected to this project.
                            </div>
                        )}
                        <MeshGraph
                            machines={state.snapshot.machines ?? []}
                            meshNodes={state.snapshot.mesh_nodes ?? []}
                            links={state.snapshot.links ?? []}
                            linkRates={state.linkRates}
                            onSelectMachine={setSelectedMachine}
                            onSelectLink={setSelectedLink}
                        />
                    </div>
                </Card>
            )}

            <MachineDetailPanel
                machine={machine}
                series={machine ? state.machineHistory.get(machine.host_id) : undefined}
                liveSessions={machine?.sessions ?? []}
                onClose={() => setSelectedMachine(null)}
            />
            <LinkDetailPanel
                projectId={project.id}
                link={link}
                rate={link ? state.linkRates.get(selectedLink ?? "") : undefined}
                onClose={() => setSelectedLink(null)}
            />
        </div>
    );
}
