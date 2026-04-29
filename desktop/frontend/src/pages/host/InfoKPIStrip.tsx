import { type ReactNode } from "react";

import AutoGrid from "../../components/AutoGrid";
import InlineBar from "../../components/InlineBar";
import MetricCard from "../../components/MetricCard";
import { Host, HostSysInfo } from "../../lib/api";

import { formatBytes, renderUptime } from "./format";

interface Props {
    host: Host;
    sysInfo: HostSysInfo | null;
}

interface Tile {
    key: string;
    label: string;
    value: ReactNode;
    hint?: ReactNode;
    accent?: "default" | "success" | "warning" | "danger";
}

// InfoKPIStrip renders the at-a-glance health row above the Info-tab
// detail grid. The five tiles cover the questions an operator asks
// in the first second after opening a host: is it pegged, out of
// memory, swapping, full, and how long has it been up?
//
// Cells render only when their underlying field is present so an
// offline agent produces a 0-tile strip rather than a row of "—"
// placeholders. The `auto-fit` grid wraps gracefully on narrow
// viewports (a 720 px sidebar+main layout collapses 5 tiles into a
// 3+2 row pair).
export default function InfoKPIStrip({ host, sysInfo }: Props) {
    const tiles = collectTiles(host, sysInfo);
    if (tiles.length === 0) return null;
    return (
        <AutoGrid minSize={140} gap={3} data-testid="host-info-kpi-strip">
            {tiles.map((t) => (
                <MetricCard
                    key={t.key}
                    label={t.label}
                    value={t.value}
                    hint={t.hint}
                    accent={t.accent}
                />
            ))}
        </AutoGrid>
    );
}

function collectTiles(host: Host, sysInfo: HostSysInfo | null): Tile[] {
    const tiles: Tile[] = [];

    // CPU %. Warn at 70, danger at 90 — InlineBar already picks the
    // right tone for the bar, but we echo it onto MetricCard's accent
    // so the value text colour matches.
    if (sysInfo?.cpu_percent !== undefined) {
        const pct = sysInfo.cpu_percent;
        tiles.push({
            key: "cpu",
            label: "CPU",
            value: (
                <InlineBar
                    value={pct}
                    width={120}
                    label="CPU usage"
                    data-testid="host-cpu-bar"
                />
            ),
            accent: pct >= 90 ? "danger" : pct >= 70 ? "warning" : "default",
        });
    }

    // Memory %. Computed from used/total — neither field alone is
    // useful as a KPI on its own (raw bytes scale with the host),
    // so we hide the tile unless both are present.
    const memTotal = sysInfo?.mem_total || host.mem_total_bytes;
    if (sysInfo?.mem_used !== undefined && memTotal) {
        const pct = (sysInfo.mem_used / memTotal) * 100;
        tiles.push({
            key: "mem",
            label: "Memory",
            value: (
                <InlineBar
                    value={pct}
                    width={120}
                    label="Memory usage"
                    data-testid="host-mem-bar"
                />
            ),
            hint: `${formatBytes(sysInfo.mem_used)} / ${formatBytes(memTotal)}`,
            accent: pct >= 90 ? "danger" : pct >= 70 ? "warning" : "default",
        });
    }

    // Load 1. Threshold is per-CPU because raw load1 means nothing
    // without core count — load1=4 on a 32-core box is idle, on a
    // 2-core box it's a full pegging.
    if (sysInfo?.load1 !== undefined) {
        const cores = sysInfo.num_cpu || host.num_cpu || 1;
        const ratio = sysInfo.load1 / cores;
        tiles.push({
            key: "load1",
            label: "Load 1m",
            value: sysInfo.load1.toFixed(2),
            hint:
                sysInfo.load5 !== undefined && sysInfo.load15 !== undefined
                    ? `${sysInfo.load5.toFixed(2)} · ${sysInfo.load15.toFixed(2)}`
                    : undefined,
            accent: ratio >= 2 ? "danger" : ratio >= 1 ? "warning" : "default",
        });
    }

    // Disk %. Pick the worst-utilised mount across all reported
    // disks. The hint names that mount so the operator can drill in
    // without having to scan the Storage table.
    if (sysInfo?.disks && sysInfo.disks.length > 0) {
        let worst: { pct: number; mount: string } | null = null;
        for (const d of sysInfo.disks) {
            if (!d.total_bytes || d.total_bytes <= 0) continue;
            const pct = ((d.used_bytes ?? 0) / d.total_bytes) * 100;
            if (!worst || pct > worst.pct) {
                worst = { pct, mount: d.mountpoint || "—" };
            }
        }
        if (worst) {
            tiles.push({
                key: "disk",
                label: "Disk",
                value: (
                    <InlineBar
                        value={worst.pct}
                        width={120}
                        label="Disk usage"
                        data-testid="host-disk-bar"
                    />
                ),
                hint: worst.mount,
                accent:
                    worst.pct >= 90 ? "danger" : worst.pct >= 80 ? "warning" : "default",
            });
        }
    }

    // Uptime. Reuses renderUptime() — already returns "Nd Nh Nm" or
    // "—". We only push the tile when the renderer would produce a
    // real string so the strip stays clean on never-seen hosts.
    const uptime = renderUptime(
        sysInfo?.uptime_seconds,
        sysInfo?.boot_time_unix || host.boot_time_unix,
    );
    if (uptime !== "—") {
        tiles.push({ key: "uptime", label: "Uptime", value: uptime });
    }

    return tiles;
}
