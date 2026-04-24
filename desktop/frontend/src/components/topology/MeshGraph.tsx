// MeshGraph — the cytoscape host component.
//
// Props are the minimum the graph needs to render: machines,
// mesh-node refs, links, and the derived per-edge rate map. The
// parent owns all state; this component only maps props → cytoscape
// elements + style.
//
// Layout is run once on mount and only re-run when the set of
// elements changes (id hash differs); stats ticks mutate element
// data in place and let the stylesheet recompute widths/colours.

import { useEffect, useMemo, useRef } from "react";
import cytoscape, { Core, ElementDefinition } from "cytoscape";
import fcose from "cytoscape-fcose";

import {
    TopologyMachine,
    TopologyMeshNodeRef,
    TopologyLink,
} from "../../lib/api";
import {
    topologyStylesheet,
    cpuBorderColor,
    edgeWidthForRate,
    edgeColorForRTT,
} from "./style";
import type { LinkRate } from "../../pages/hooks/useTopology";

// One-shot registration. Cytoscape throws if you register the same
// layout extension twice, so guard with a module-level flag.
let fcoseRegistered = false;
function ensureFcose() {
    if (fcoseRegistered) return;
    cytoscape.use(fcose);
    fcoseRegistered = true;
}

export interface MeshGraphProps {
    machines: TopologyMachine[];
    meshNodes: TopologyMeshNodeRef[];
    links: TopologyLink[];
    linkRates: Map<string, LinkRate>;
    onSelectMachine?: (hostID: string | null) => void;
    onSelectLink?: (edgeKey: string | null) => void;
}

// elementsFor builds the cytoscape element array from the snapshot.
// Machines become compound parents; sessions / mesh nodes become
// children or free-floating diamonds as appropriate.
function elementsFor(props: MeshGraphProps): ElementDefinition[] {
    const { machines, meshNodes, links, linkRates } = props;
    const out: ElementDefinition[] = [];

    const meshNodesByHost = new Map<string, TopologyMeshNodeRef>();
    for (const r of meshNodes) {
        if (r.host_id) meshNodesByHost.set(r.host_id, r);
    }

    // 1. Machine compound parents + their session / mesh children.
    for (const m of machines) {
        const cpu = m.sys_info?.cpu_percent;
        out.push({
            group: "nodes",
            data: {
                id: `machine:${m.host_id}`,
                type: "machine",
                label: m.hostname || m.host_id.slice(0, 8),
                hostId: m.host_id,
                borderColor: cpuBorderColor(cpu),
            },
        });
        for (const s of m.sessions) {
            out.push({
                group: "nodes",
                data: {
                    id: `session:${s.id}`,
                    parent: `machine:${m.host_id}`,
                    type: "session",
                    label: s.user ?? "",
                    bg: s.active ? "#0070f3" : "#525252",
                    border: s.active ? "#60a5fa" : "#262626",
                    sessionId: s.id,
                    active: s.active,
                },
            });
        }
        const mn = meshNodesByHost.get(m.host_id);
        if (mn) {
            out.push({
                group: "nodes",
                data: {
                    id: `mesh:${mn.node_id}`,
                    parent: `machine:${m.host_id}`,
                    type: "mesh",
                    label: mn.node_id.slice(0, 8),
                    nodeId: mn.node_id,
                },
            });
        }
    }

    // 2. Orphan mesh nodes (no project host_id) — typically the
    //    server's own node, or peers learnt via LSDB.
    for (const r of meshNodes) {
        if (r.host_id) continue;
        out.push({
            group: "nodes",
            data: {
                id: `mesh:${r.node_id}`,
                type: "mesh",
                label: r.kind === "self" ? "server" : r.node_id.slice(0, 8),
                nodeId: r.node_id,
            },
        });
    }

    // 3. Edges. Edge ID is the canonical pair key so in-place data
    //    updates are easy to route.
    for (const l of links) {
        const key = l.a < l.b ? `${l.a}|${l.b}` : `${l.b}|${l.a}`;
        const rate = linkRates.get(key);
        const bytesRate = rate ? rate.bytesInPerSec + rate.bytesOutPerSec : 0;
        const rttMs = rate?.rttMs ?? (l.rtt_ns ? l.rtt_ns / 1e6 : undefined);
        out.push({
            group: "edges",
            data: {
                id: `edge:${key}`,
                source: `mesh:${l.a}`,
                target: `mesh:${l.b}`,
                up: l.up ? "up" : "down",
                width: edgeWidthForRate(bytesRate),
                color: l.up ? edgeColorForRTT(rttMs) : "#4b5563",
                label: rate && bytesRate > 128 ? formatRate(bytesRate) : "",
                bytesRate,
                rttMs: rttMs ?? 0,
            },
        });
    }

    return out;
}

function formatRate(bps: number): string {
    if (bps < 1024) return `${bps.toFixed(0)} B/s`;
    if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(1)} KB/s`;
    return `${(bps / (1024 * 1024)).toFixed(1)} MB/s`;
}

// idHash is the content hash of element ids. Used to decide whether
// a layout re-run is necessary — rates-only updates don't change
// this.
function idHash(els: ElementDefinition[]): string {
    return els.map((e) => e.data.id).sort().join(",");
}

export default function MeshGraph(props: MeshGraphProps) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const cyRef = useRef<Core | null>(null);
    const lastIdHashRef = useRef<string>("");

    const elements = useMemo(() => elementsFor(props), [props]);
    const currentHash = useMemo(() => idHash(elements), [elements]);

    // Mount once.
    useEffect(() => {
        ensureFcose();
        if (!containerRef.current) return;
        const cy = cytoscape({
            container: containerRef.current,
            elements,
            style: topologyStylesheet(),
            // wheelSensitivity is deliberately left at Cytoscape's
            // default — overriding it makes the library log a "custom
            // wheel sensitivity" warning on every mount, and our zoom
            // already feels fine out of the box.
            minZoom: 0.25,
            maxZoom: 2.5,
        });
        cyRef.current = cy;
        runLayout(cy);
        lastIdHashRef.current = currentHash;

        cy.on("tap", 'node[type="machine"]', (evt) => {
            props.onSelectMachine?.(evt.target.data("hostId"));
        });
        cy.on("tap", "edge", (evt) => {
            props.onSelectLink?.(evt.target.data("id").replace(/^edge:/, ""));
        });
        cy.on("tap", (evt) => {
            if (evt.target === cy) {
                props.onSelectMachine?.(null);
                props.onSelectLink?.(null);
            }
        });

        return () => {
            cy.destroy();
            cyRef.current = null;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Reconcile on prop changes.
    useEffect(() => {
        const cy = cyRef.current;
        if (!cy) return;
        cy.batch(() => {
            // Diff: add new elements, update data on existing, remove missing.
            const keep = new Set(elements.map((e) => e.data.id).filter(Boolean) as string[]);
            cy.elements().forEach((el) => {
                if (!keep.has(el.data("id"))) cy.remove(el);
            });
            for (const el of elements) {
                const id = el.data.id;
                if (!id) continue;
                const existing = cy.getElementById(id);
                if (existing.nonempty()) {
                    existing.data(el.data);
                } else {
                    cy.add(el);
                }
            }
        });
        if (currentHash !== lastIdHashRef.current) {
            runLayout(cy);
            lastIdHashRef.current = currentHash;
        }
    }, [elements, currentHash]);

    return (
        <div
            ref={containerRef}
            style={{
                width: "100%",
                height: "100%",
                minHeight: 400,
                background: "transparent",
            }}
        />
    );
}

function runLayout(cy: Core) {
    const layout = cy.layout({
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        name: "fcose" as any,
        animate: false,
        quality: "default",
        nodeRepulsion: 4000,
        idealEdgeLength: 80,
        nodeSeparation: 50,
        padding: 30,
        randomize: false,
        fit: true,
    } as cytoscape.LayoutOptions);
    layout.run();
}
