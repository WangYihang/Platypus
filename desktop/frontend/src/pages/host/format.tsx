import type { ReactNode } from "react";

import Mono from "../../components/Mono";

// Shared formatters for the Host detail surface (Info tab + cards).
// Local to pages/host/* because the table view (Fleet) renders the
// same fields with different humanisation (e.g. shorter "GB" vs.
// detailed "X / Y · Z %") and we don't want a single global helper
// to drift in either direction.

// formatBytes turns a byte count into a short human-friendly label
// (e.g. "124 GB"). Returns "—" for undefined / zero so tables align.
export function formatBytes(n?: number): string {
    if (!n || n <= 0) return "—";
    const units = ["B", "KB", "MB", "GB", "TB", "PB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

export function formatPercent(used?: number, total?: number): string {
    if (!used || !total || total <= 0) return "—";
    return `${((used / total) * 100).toFixed(1)} %`;
}

export function renderMemoryLine(
    used?: number,
    total?: number,
    available?: number,
): ReactNode {
    if (!total) return "—";
    const pct = used ? ` · ${((used / total) * 100).toFixed(1)} %` : "";
    return (
        <span>
            {formatBytes(used)} / {formatBytes(total)}
            {pct}
            {available ? ` · ${formatBytes(available)} avail` : ""}
        </span>
    );
}

export function renderLoadLine(
    l1?: number,
    l5?: number,
    l15?: number,
): ReactNode {
    if (l1 === undefined && l5 === undefined && l15 === undefined) return "—";
    const fmt = (n?: number) => (n === undefined ? "—" : n.toFixed(2));
    return (
        <Mono>
            {fmt(l1)} · {fmt(l5)} · {fmt(l15)}
        </Mono>
    );
}

export function renderUptime(secs?: number, bootUnix?: number): ReactNode {
    if (!secs && !bootUnix) return "—";
    const s =
        secs ?? (bootUnix ? Math.max(0, Math.floor(Date.now() / 1000) - bootUnix) : 0);
    if (!s) return "—";
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    const parts: string[] = [];
    if (d) parts.push(`${d}d`);
    if (h || d) parts.push(`${h}h`);
    parts.push(`${m}m`);
    return parts.join(" ");
}
