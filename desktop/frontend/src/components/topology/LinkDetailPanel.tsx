// LinkDetailPanel — right-side Sheet for an edge. Shows live
// counters + RTT, plus a "load last hour" button that queries the
// history stats endpoint and renders it with recharts.

import { useState } from "react";
import { Line, LineChart, ResponsiveContainer, XAxis, YAxis } from "recharts";

import { Button } from "@/components/ui/button";
import {
    Sheet,
    SheetContent,
    SheetDescription,
    SheetHeader,
    SheetTitle,
} from "@/components/ui/sheet";
import { Separator } from "@/components/ui/separator";

import { palette } from "../../layout/theme";
import { TopologyLink, LinkHistoryPoint, fetchLinkHistory } from "../../lib/api";
import type { LinkRate } from "../../pages/hooks/useTopology";

export interface LinkDetailPanelProps {
    projectId: string;
    link: TopologyLink | null;
    rate: LinkRate | undefined;
    onClose: () => void;
}

function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
    if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MiB`;
    return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
}

function formatRate(bps: number): string {
    if (bps < 1024) return `${bps.toFixed(0)} B/s`;
    if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(1)} KiB/s`;
    return `${(bps / (1024 * 1024)).toFixed(2)} MiB/s`;
}

export default function LinkDetailPanel({
    projectId,
    link,
    rate,
    onClose,
}: LinkDetailPanelProps) {
    const [history, setHistory] = useState<LinkHistoryPoint[] | null>(null);
    const [historyLoading, setHistoryLoading] = useState(false);

    async function loadHistory() {
        if (!link) return;
        setHistoryLoading(true);
        try {
            const until = new Date();
            const since = new Date(until.getTime() - 60 * 60 * 1000);
            const pts = await fetchLinkHistory(projectId, link.a, link.b, {
                since,
                until,
                max: 300,
            });
            setHistory(pts);
        } catch (e) {
            setHistory([]);
        } finally {
            setHistoryLoading(false);
        }
    }

    return (
        <Sheet open={!!link} onOpenChange={(o) => !o && onClose()}>
            <SheetContent className="w-[420px] sm:max-w-[420px] overflow-y-auto">
                {link && (
                    <>
                        <SheetHeader>
                            <SheetTitle className="font-mono text-sm">
                                {link.a.slice(0, 8)} ↔ {link.b.slice(0, 8)}
                            </SheetTitle>
                            <SheetDescription>
                                {link.up ? "connected" : "disconnected"}
                                {rate?.rttMs ? ` · ${rate.rttMs.toFixed(1)} ms RTT` : ""}
                            </SheetDescription>
                        </SheetHeader>

                        <div className="px-4 pb-4 space-y-4 text-sm" style={{ color: palette.textPrimary }}>
                            <section className="grid grid-cols-2 gap-3">
                                <Metric label="In rate" value={formatRate(rate?.bytesInPerSec ?? 0)} />
                                <Metric label="Out rate" value={formatRate(rate?.bytesOutPerSec ?? 0)} />
                                <Metric label="Total in" value={formatBytes(link.bytes_in)} />
                                <Metric label="Total out" value={formatBytes(link.bytes_out)} />
                                <Metric label="Msgs in" value={link.msgs_in.toLocaleString()} />
                                <Metric label="Msgs out" value={link.msgs_out.toLocaleString()} />
                            </section>

                            <Separator />

                            <section>
                                <div className="flex items-center justify-between mb-2">
                                    <span className="text-xs" style={{ color: palette.textMuted }}>Last hour</span>
                                    <Button size="sm" variant="outline" onClick={loadHistory} disabled={historyLoading}>
                                        {historyLoading ? "Loading..." : history ? "Reload" : "Load"}
                                    </Button>
                                </div>
                                {history && history.length > 0 && (
                                    <div className="h-36">
                                        <ResponsiveContainer width="100%" height="100%" minWidth={0} minHeight={0}>
                                            <LineChart
                                                data={history.map((p, i) => {
                                                    if (i === 0) return { t: p.at, rate: 0 };
                                                    const prev = history[i - 1];
                                                    const dt = Math.max(1, (new Date(p.at).getTime() - new Date(prev.at).getTime()) / 1000);
                                                    const delta = Math.max(0, (p.bytes_in + p.bytes_out) - (prev.bytes_in + prev.bytes_out));
                                                    return { t: p.at, rate: delta / dt };
                                                })}
                                                margin={{ top: 5, right: 5, bottom: 5, left: 5 }}
                                            >
                                                <XAxis dataKey="t" hide />
                                                <YAxis hide />
                                                <Line
                                                    type="monotone"
                                                    dataKey="rate"
                                                    stroke="#0070f3"
                                                    strokeWidth={1.5}
                                                    dot={false}
                                                    isAnimationActive={false}
                                                />
                                            </LineChart>
                                        </ResponsiveContainer>
                                    </div>
                                )}
                                {history && history.length === 0 && (
                                    <p className="text-xs" style={{ color: palette.textMuted }}>
                                        No samples in the window.
                                    </p>
                                )}
                            </section>
                        </div>
                    </>
                )}
            </SheetContent>
        </Sheet>
    );
}

function Metric({ label, value }: { label: string; value: string }) {
    return (
        <div>
            <div className="text-xs" style={{ color: palette.textMuted }}>{label}</div>
            <div className="font-mono">{value}</div>
        </div>
    );
}
