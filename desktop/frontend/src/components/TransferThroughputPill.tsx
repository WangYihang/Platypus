import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDownToLine, ArrowUpFromLine } from "lucide-react";

import { palette, radius, space } from "../layout/theme";
import {
    computeInstantaneousRate,
    formatBytesPerSec,
    pruneSamplesOlderThan,
    type ThroughputSample,
    type TransferItem,
    transferDirectionTone,
    transferProgressPct,
} from "../lib/transfers";
import { useTransfersDrawer } from "./TransfersPill";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "@/components/ui/popover";

// Window over which we average bytes/sec. 5 s is wide enough that
// short backpressure stalls don't make the pill jitter to 0, narrow
// enough that the operator sees activity changes within a couple of
// updates rather than staring at a stale average.
const WINDOW_MS = 5_000;

// formatBytes mirrors lib/format's helper; duplicating here keeps
// the throughput pill's bundle dep-free of the broader format module
// (which pulls in dayjs).
function formatBytes(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let idx = 0;
    let v = n;
    while (v >= 1024 && idx < units.length - 1) {
        v /= 1024;
        idx++;
    }
    return `${v.toFixed(v >= 100 || idx === 0 ? 0 : 1)} ${units[idx]}`;
}

function shortPath(p: string): string {
    if (p.length <= 36) return p;
    // Keep the basename — operators care about which file is moving.
    const slash = p.lastIndexOf("/");
    const basename = slash >= 0 ? p.slice(slash) : p;
    return `…${basename.slice(-32)}`;
}

/**
 * TransferThroughputPill renders a status-bar chip showing two
 * numbers: the instantaneous bytes/sec across all in-flight transfers
 * and the cumulative bytes processed since the pill mounted (page
 * load). Hover opens a Popover listing every running transfer with
 * its individual throughput contribution.
 *
 * The pill subscribes to the global transfers store via the existing
 * `useTransfersDrawer()` context so it doesn't need its own store
 * instance — the drawer's rows ARE the source of truth.
 *
 * Rate calculation is a 5 s sliding window: every store update pushes
 * `{ ts: now, bytes: totalSourceBytes }` into a ref-held ring; the
 * helper computeInstantaneousRate(samples) returns the oldest-vs-
 * newest slope. When the running-row count drops to 0 we reset the
 * ring so a later resume doesn't render a stale-but-stable rate.
 */
export default function TransferThroughputPill() {
    const { rows } = useTransfersDrawer();

    // Sample ring + rerender trigger. The ring lives in a ref to
    // survive renders that aren't store updates; we mirror it into
    // state so React knows when to recompute the displayed rate.
    const samplesRef = useRef<ThroughputSample[]>([]);
    const [samples, setSamples] = useState<ThroughputSample[]>([]);

    // Cumulative-since-mount baseline: the sum of bytes_transferred
    // across all rows on the very first useEffect tick. After that
    // we render `current - baseline`.
    const baselineRef = useRef<number | null>(null);

    const totalSourceBytes = useMemo(
        () => rows.reduce((s, r) => s + r.bytes_transferred, 0),
        [rows],
    );
    const runningRows = useMemo(
        () => rows.filter((r) => r.status === "running" || r.status === "pending"),
        [rows],
    );

    useEffect(() => {
        if (baselineRef.current === null) {
            baselineRef.current = totalSourceBytes;
        }
        // No active transfers → flush the ring so the next session
        // starts fresh. Keeps "rate" honest: an idle pill must read 0,
        // not the average from the last burst.
        if (runningRows.length === 0) {
            samplesRef.current = [];
            setSamples([]);
            return;
        }
        const now = Date.now();
        const next = pruneSamplesOlderThan(
            [...samplesRef.current, { ts: now, bytes: totalSourceBytes }],
            now,
            WINDOW_MS,
        );
        samplesRef.current = next;
        setSamples(next);
    }, [totalSourceBytes, runningRows.length]);

    const rate = computeInstantaneousRate(samples);
    const active = runningRows.length > 0;
    const cumulativeBytes =
        baselineRef.current !== null
            ? Math.max(0, totalSourceBytes - baselineRef.current)
            : 0;

    const rateText = active && rate !== null ? formatBytesPerSec(rate) : "—";
    const cumulativeText = active ? formatBytes(cumulativeBytes) : "";

    return (
        <Popover>
            <PopoverTrigger asChild>
                <button
                    type="button"
                    data-testid="transfer-throughput-pill"
                    data-active={active ? "true" : "false"}
                    aria-label={
                        active
                            ? `${rateText} across ${runningRows.length} transfer${
                                  runningRows.length === 1 ? "" : "s"
                              }`
                            : "No active transfers"
                    }
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 5,
                        padding: "2px 10px",
                        background: active ? palette.infoSoft : palette.surface,
                        border: `1px solid ${active ? palette.info : palette.border}`,
                        borderRadius: radius.pill,
                        color: active ? palette.info : palette.textSecondary,
                        fontSize: 12,
                        fontWeight: active ? 600 : 400,
                        cursor: "pointer",
                        fontVariantNumeric: "tabular-nums",
                    }}
                >
                    <ArrowDownToLine
                        className="size-3"
                        style={{ color: active ? palette.success : palette.textMuted }}
                    />
                    <ArrowUpFromLine
                        className="size-3"
                        style={{ color: active ? palette.info : palette.textMuted }}
                    />
                    <span>{rateText}</span>
                    {cumulativeText ? (
                        <>
                            <span style={{ color: palette.textMuted }}>·</span>
                            <span>{cumulativeText}</span>
                        </>
                    ) : null}
                </button>
            </PopoverTrigger>
            <PopoverContent side="top" align="end" className="w-[320px] text-xs">
                <div className="space-y-2">
                    <div className="flex items-center justify-between">
                        <span className="text-text-muted">Throughput</span>
                        <span className="text-text-primary tabular-nums">
                            {rateText}
                        </span>
                    </div>
                    <div className="flex items-center justify-between">
                        <span className="text-text-muted">
                            Cumulative (this session)
                        </span>
                        <span className="text-text-primary tabular-nums">
                            {formatBytes(cumulativeBytes)}
                        </span>
                    </div>
                    {runningRows.length === 0 ? (
                        <div className="text-text-muted text-center py-2">
                            No active transfers
                        </div>
                    ) : (
                        <div
                            style={{
                                marginTop: space[2],
                                paddingTop: space[2],
                                borderTop: `1px solid ${palette.border}`,
                            }}
                        >
                            {runningRows.map((it) => (
                                <ThroughputRow key={it.id} it={it} />
                            ))}
                        </div>
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}

function ThroughputRow({ it }: { it: TransferItem }) {
    const tone = transferDirectionTone(it);
    const Icon = it.direction === "upload" ? ArrowUpFromLine : ArrowDownToLine;
    const color = tone === "info" ? palette.info : palette.success;
    const pct = transferProgressPct(it);
    const pctText = pct === null ? "" : `${pct}%`;
    const path = it.paths[0] || "";
    return (
        <div
            data-testid="throughput-row"
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: `${space[1]}px 0`,
                fontSize: 11,
            }}
        >
            <Icon
                className="size-3"
                style={{ color, flexShrink: 0 }}
                data-direction-tone={tone}
            />
            <span
                style={{
                    flex: 1,
                    minWidth: 0,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    fontFamily: "var(--font-mono, ui-monospace, monospace)",
                    color: palette.textPrimary,
                }}
                title={path}
            >
                {shortPath(path)}
            </span>
            {pctText ? (
                <span
                    style={{
                        color: palette.textMuted,
                        fontVariantNumeric: "tabular-nums",
                    }}
                >
                    {pctText}
                </span>
            ) : null}
        </div>
    );
}
