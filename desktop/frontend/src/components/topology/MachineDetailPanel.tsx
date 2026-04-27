// MachineDetailPanel — right-side Sheet that shows full host info
// when a compound machine node is selected on the graph.

import { useMemo } from "react";
import { Line, LineChart, YAxis } from "recharts";

import ChartContainer from "../charts/ChartContainer";

import { Badge } from "@/components/ui/badge";
import {
    Sheet,
    SheetContent,
    SheetDescription,
    SheetHeader,
    SheetTitle,
} from "@/components/ui/sheet";
import { Separator } from "@/components/ui/separator";
import { TopologyMachine, TopologySession } from "../../lib/api";
import { fromNow } from "../../lib/time";
import { palette } from "../../layout/theme";

interface Props {
    machine: TopologyMachine | null;
    series: { cpu: Array<{ t: number; v: number }>; mem: Array<{ t: number; v: number }> } | undefined;
    liveSessions: TopologySession[];
    onClose: () => void;
}

export default function MachineDetailPanel({
    machine,
    series,
    liveSessions,
    onClose,
}: Props) {
    const sys = machine?.sys_info;
    const isUp = useMemo(() => liveSessions.length > 0, [liveSessions]);

    return (
        <Sheet open={!!machine} onOpenChange={(open) => !open && onClose()}>
            <SheetContent className="w-[420px] sm:max-w-[420px] overflow-y-auto">
                {machine && (
                    <>
                        <SheetHeader className="mb-6">
                            <div className="flex items-center gap-2 mb-1">
                                <div
                                    className="size-2.5 rounded-full"
                                    style={{
                                        background: isUp
                                            ? palette.info
                                            : palette.textMuted,
                                    }}
                                />
                                <SheetTitle className="font-mono text-lg">
                                    {machine.hostname || machine.host_id.slice(0, 12)}
                                </SheetTitle>
                            </div>
                            <SheetDescription className="flex items-center gap-2">
                                <Badge variant="outline" className="font-mono font-normal">
                                    {machine.host_id.slice(0, 8)}
                                </Badge>
                                <span>•</span>
                                <span>{liveSessions.length} active sessions</span>
                            </SheetDescription>
                        </SheetHeader>

                        <div className="space-y-8">
                            <section>
                                <h3 className="text-sm font-medium mb-4">System Stats</h3>
                                <div className="grid grid-cols-2 gap-4">
                                    <div className="space-y-1">
                                        <div className="flex items-center justify-between text-[11px] uppercase tracking-wider text-muted-foreground">
                                            <span>CPU</span>
                                            <span>{sys?.cpu_percent?.toFixed(1) ?? 0}%</span>
                                        </div>
                                        <div className="h-10 bg-muted/30 rounded-md overflow-hidden">
                                            <Sparkline data={series?.cpu ?? []} />
                                        </div>
                                    </div>
                                    <div className="space-y-1">
                                        <div className="flex items-center justify-between text-[11px] uppercase tracking-wider text-muted-foreground">
                                            <span>Memory</span>
                                            <span>{sys?.mem_percent?.toFixed(1) ?? 0}%</span>
                                        </div>
                                        <div className="h-10 bg-muted/30 rounded-md overflow-hidden">
                                            <Sparkline data={series?.mem ?? []} />
                                        </div>
                                    </div>
                                </div>
                            </section>

                            <Separator />

                            <section>
                                <h3 className="text-sm font-medium mb-4">Host Information</h3>
                                <dl className="grid grid-cols-[100px_1fr] gap-y-3">
                                    <Kv k="OS" v={machine.os} />
                                    <Kv k="Platform" v={sys?.platform} />
                                    <Kv k="Kernel" v={sys?.kernel_version} />
                                    <Kv k="Platform Ver" v={sys?.platform_version} />
                                    <Kv k="Last Seen" v={fromNow(machine.last_seen_at)} />
                                </dl>
                            </section>

                            {(sys?.mem_total_bytes ?? 0) > 0 && (
                                <>
                                    <Separator />
                                    <section>
                                        <h3 className="text-sm font-medium mb-3">
                                            Hardware
                                        </h3>
                                        <dl className="grid grid-cols-[100px_1fr] gap-y-3">
                                            <Kv k="Total RAM" v={`${((sys?.mem_total_bytes ?? 0) / 1024 / 1024 / 1024).toFixed(1)} GiB`} />
                                            <Kv k="Uptime" v={`${((sys?.uptime_seconds ?? 0) / 3600).toFixed(1)} hours`} />
                                        </dl>
                                    </section>
                                </>
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
        <ChartContainer height="100%">
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
        </ChartContainer>
    );
}
