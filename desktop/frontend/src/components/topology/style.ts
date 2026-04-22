// Topology stylesheet — consumed by MeshGraph.tsx. Kept in its own
// file to make it easy to iterate on edge/node visual treatment
// without touching the imperative cytoscape wiring.

import { palette } from "../../layout/theme";

// topologyStylesheet returns a cytoscape stylesheet pinned to the
// app's dark-mode palette. Selectors target the element types we
// emit: compound "machine" parents, session + mesh nodes, and edges.
//
// Cytoscape's published typings are stricter than the runtime — it
// happily accepts numeric `border-width: 1.5`, `padding: 12`, etc. —
// so we return an untyped array and let the cytoscape call-site
// widen the argument. Keeps this file uncluttered by per-value
// string coercions.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function topologyStylesheet(): any[] {
    return [
        // Compound machine node (parent wrapping session + mesh children).
        {
            selector: 'node[type="machine"]',
            style: {
                "background-color": palette.surface,
                "background-opacity": 0.85,
                "border-color": "data(borderColor)",
                "border-width": 1.5,
                "shape": "round-rectangle",
                "padding": 12,
                "label": "data(label)",
                "color": palette.textPrimary,
                "font-size": 12,
                "font-weight": 600,
                "text-valign": "top",
                "text-halign": "center",
                "text-margin-y": -6,
                "compound-sizing-wrt-labels": "include",
            },
        },
        // Session child.
        {
            selector: 'node[type="session"]',
            style: {
                "background-color": "data(bg)",
                "border-color": "data(border)",
                "border-width": 1,
                "shape": "ellipse",
                "width": 18,
                "height": 18,
                "label": "data(label)",
                "color": palette.textMuted,
                "font-size": 9,
                "text-valign": "bottom",
                "text-margin-y": 2,
            },
        },
        // Mesh node (used when rendering mesh NodeIDs that don't
        // belong to a project machine — e.g. the server in mesh
        // mode). Kept visually distinct from machine compounds.
        {
            selector: 'node[type="mesh"]',
            style: {
                "background-color": "#1f2937",
                "border-color": "#60a5fa",
                "border-width": 2,
                "shape": "diamond",
                "width": 26,
                "height": 26,
                "label": "data(label)",
                "color": palette.textPrimary,
                "font-size": 10,
                "text-valign": "bottom",
                "text-margin-y": 3,
            },
        },
        // Generic edge.
        {
            selector: "edge",
            style: {
                "width": "data(width)",
                "line-color": "data(color)",
                "opacity": 0.85,
                "curve-style": "bezier",
                "target-arrow-shape": "none",
                "label": "data(label)",
                "font-size": 9,
                "color": palette.textMuted,
                "text-rotation": "autorotate",
                "text-background-color": palette.surface,
                "text-background-opacity": 0.7,
                "text-background-padding": 2,
            },
        },
        {
            selector: 'edge[up="down"]',
            style: {
                "line-style": "dashed",
                "opacity": 0.5,
            },
        },
        // Selection affordance.
        {
            selector: ":selected",
            style: {
                "border-color": "#60a5fa",
                "border-width": 3,
                "overlay-color": "#60a5fa",
                "overlay-opacity": 0.1,
            },
        },
    ];
}

// Map CPU% to a tint used as the machine node border colour.
//   < 30%  → green
//   30-70 → yellow
//   > 70%  → red
export function cpuBorderColor(cpu: number | undefined): string {
    if (cpu === undefined) return palette.border ?? "#333";
    if (cpu < 30) return "#22c55e";
    if (cpu < 70) return "#eab308";
    return "#ef4444";
}

// Edge width grows with log(rate). Clamped so a heavy link doesn't
// visually dominate the whole graph.
export function edgeWidthForRate(bytesPerSec: number): number {
    if (!isFinite(bytesPerSec) || bytesPerSec <= 0) return 1;
    const kb = bytesPerSec / 1024;
    return Math.min(8, 1 + Math.log10(1 + kb));
}

// Edge color bucketed by RTT (higher = warmer).
export function edgeColorForRTT(rttMs: number | undefined): string {
    if (rttMs === undefined || rttMs <= 0) return "#9ca3af"; // unknown
    if (rttMs < 20) return "#22c55e";
    if (rttMs < 80) return "#eab308";
    return "#ef4444";
}
