// MachineDetailPanel — right-side Sheet that shows full host info
// when a compound machine node is selected on the graph.

import { useMemo } from "react";
import { Line, LineChart, ResponsiveContainer, YAxis } from "recharts";

import { Badge } from "@/components/ui/badge";
import {
    Sheet,
    SheetContent,
    SheetDescription,
    SheetHeader,
    SheetTitle,
} from "@/components/ui/sheet";
import { Separator } from "@/components/ui/separator";

import { palette } from "../../layout/theme";
import { TopologyMachine } from "../../lib/api";
import type { MachineSeries } from "../../pages/hooks/useTopology";

export interface MachineDetailPanelProps {
    machine: TopologyMachine | null;
    series: MachineSeries | undefined;
    onClose: () => void;
}

function formatBytes(n?: number): string {
    if (!n) return "—";
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    let v = n;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i += 1;
    }
    return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`;
}

function formatUptime(s?: number): string {
    if (!s || s < 0) return "—";
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
}

export default function MachineDetailPanel({
    machine,
    series,
    onClose,
}: MachineDetailPanelProps) {
    const activeSessions = useMemo(
        () => (machine?.sessions ?? []).filter((s) => s.active),
        [machine],
    );
    const historicalSessions = useMemo(
        () => (machine?.sessions ?? []).filter((s) => !s.active),
        [machine],
    );

    return (
        <Sheet open={!!machine} onOpenChange={(o) => !o && onClose()}>
            <SheetContent className="w-[420px] sm:max-w-[420px] overflow-y-auto">
                {machine && (
                    <>
                        <SheetHeader>
                            <SheetTitle>{machine.hostname || machine.host_id.slice(0, 12)}</SheetTitle>
                            <SheetDescription>{machine.os || "unknown OS"}</SheetDescription>
                        </SheetHeader>

                        <div className="px-4 pb-4 space-y-4 text-sm" style={{ color: palette.textPrimary }}>
                            <section>
                                <div className="flex items-center justify-between">
                                    <span className="text-xs" style={{ color: palette.textMuted }}>CPU</span>
                                    <span>{machine.sys_info?.cpu_percent?.toFixed(1) ?? "—"}%</span>
                                </div>
                                <div className="h-10">
                                    <Sparkline data={series?.cpu ?? []} />
                                </div>
                            </section>
                            <section>
                                <div className="flex items-center justify-between">
                                    <span className="text-xs" style={{ color: palette.textMuted }}>Memory</span>
                                    <span>
                                        {machine.sys_info?.mem_percent?.toFixed(1) ?? "—"}%
                                        <span className="ml-1 text-xs" style={{ color: palette.textMuted }}>
                                            ({formatBytes(machine.sys_info?.mem_used_bytes)} /{" "}
                                            {formatBytes(machine.sys_info?.mem_total_bytes)})
                                        </span>
                                    </span>
                                </div>
                                <div className="h-10">
                                    <Sparkline data={series?.mem ?? []} />
                                </div>
                            </section>

                            <Separator />

                            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                                <Kv k="Distribution" v={machine.sys_info?.os_distribution} />
                                <Kv k="Kernel" v={machine.sys_info?.kernel_version} />
                                <Kv k="Uptime" v={formatUptime(machine.sys_info?.uptime_seconds)} />
                                <Kv k="Machine ID" v={machine.machine_id} mono />
                                <Kv k="Fingerprint" v={machine.fingerprint} mono />
                                <Kv k="First seen" v={new Date(machine.first_seen_at).toLocaleString()} />
                                <Kv k="Last seen" v={new Date(machine.last_seen_at).toLocaleString()} />
                            </dl>

                            <Separator />

                            <section>
                                <h4 className="text-xs font-semibold mb-2" style={{ color: palette.textMuted }}>
                                    Active sessions ({activeSessions.length})
                                </h4>
                                {activeSessions.length === 0 && (
                                    <p className="text-xs" style={{ color: palette.textMuted }}>
                                        None connected.
                                    </p>
                                )}
                                {activeSessions.map((s) => (
                                    <div key={s.id} className="flex items-center justify-between py-1">
                                        <div className="truncate">
                                            <Badge variant="default">live</Badge>
                                            <span className="ml-2">{s.user ?? "—"}</span>
                                            <span className="ml-2 text-xs" style={{ color: palette.textMuted }}>
                                                {s.remote_addr}
                                            </span>
                                        </div>
                                    </div>
                                ))}
                            </section>

                            {historicalSessions.length > 0 && (
                                <section>
                                    <h4 className="text-xs font-semibold mb-2" style={{ color: palette.textMuted }}>
                                        Historical sessions ({historicalSessions.length})
                                    </h4>
                                    <div className="max-h-48 overflow-y-auto text-xs space-y-1">
                                        {historicalSessions.slice(0, 20).map((s) => (
                                            <div key={s.id} className="flex items-center justify-between">
                                                <span style={{ color: palette.textMuted }}>
                                                    {new Date(s.connected_at).toLocaleString()}
                                                </span>
                                                <span>{s.user ?? "—"}</span>
                                            </div>
                                        ))}
                                    </div>
                                </section>
                            )}
                        </div>
                    </>
                )}
            </SheetContent>
        </Sheet>
    );
}

function Kv({ k, v, mono }: { k: string; v?: string; mono?: boolean }) {
    return (
        <>
            <dt className="text-xs" style={{ color: palette.textMuted }}>{k}</dt>
            <dd className={mono ? "font-mono text-xs truncate" : "text-xs"}>{v ?? "—"}</dd>
        </>
    );
}

function Sparkline({ data }: { data: Array<{ t: number; v: number }> }) {
    if (data.length === 0) {
        return <div className="h-full" style={{ borderLeft: `1px solid ${palette.border}` }} />;
    }
    return (
        <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 2, right: 2, bottom: 2, left: 2 }}>
                <YAxis hide domain={[0, 100]} />
                <Line
                    type="monotone"
                    dataKey="v"
                    stroke="#0070f3"
                    strokeWidth={1.5}
                    dot={false}
                    isAnimationActive={false}
                />
            </LineChart>
        </ResponsiveContainer>
    );
}
