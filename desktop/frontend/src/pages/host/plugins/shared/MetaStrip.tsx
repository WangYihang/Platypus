// <MetaStrip> — the "Updated 12s ago · ⟳ · 30s ▾" strip rendered
// above any tabular / live data view. Originally lived inside
// RPCTable; lifted here so the builtin plugin tabs (Processes,
// Config, Security, …) which don't go through RPCTable can use
// the same primitive.
//
// Three slots, left → right:
//   - relative "Updated …" indicator, sourced from the caller's
//     query.dataUpdatedAt and ticked every 5s so the string stays
//     current between refetches.
//   - manual refresh button (RefreshCw icon, spins while fetching).
//   - auto-refresh interval picker (Off / 5s / 10s / 15s / 30s /
//     1m / 5m). The choice is owned by the caller via the
//     `useRefreshInterval` hook, which transparently persists per
//     (pluginID, agentID) to localStorage so it survives reloads.

import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { palette, space } from "../../../../layout/theme";

// 15s is included so Network (refreshMs=15000) lands on a fixed
// option rather than rendering as a blank <select>; 30s/60s cover
// Services and Filesystems respectively. If a caller's default
// drifts outside this set, MetaStrip falls back to appending the
// orphan value as a synthetic option.
export const INTERVAL_OPTIONS: ReadonlyArray<{ ms: number; label: string }> = [
    { ms: 0, label: "Off" },
    { ms: 5000, label: "5s" },
    { ms: 10000, label: "10s" },
    { ms: 15000, label: "15s" },
    { ms: 30000, label: "30s" },
    { ms: 60000, label: "1m" },
    { ms: 300000, label: "5m" },
];

const REFRESH_STORAGE_PREFIX = "rpc-refresh:";

function refreshStorageKey(pluginID: string, agentID: string): string {
    return `${REFRESH_STORAGE_PREFIX}${pluginID}:${agentID}`;
}

// localStorage may throw under private mode / quota / SSR; treat
// every failure as "no preference saved".
function readPersistedInterval(
    pluginID: string,
    agentID: string,
): number | null {
    try {
        const raw = window.localStorage.getItem(
            refreshStorageKey(pluginID, agentID),
        );
        if (raw === null) return null;
        const n = Number(raw);
        if (!Number.isFinite(n) || n < 0) return null;
        return n;
    } catch {
        return null;
    }
}

function writePersistedInterval(
    pluginID: string,
    agentID: string,
    ms: number | null,
): void {
    try {
        const key = refreshStorageKey(pluginID, agentID);
        if (ms === null) {
            window.localStorage.removeItem(key);
        } else {
            window.localStorage.setItem(key, String(ms));
        }
    } catch {
        // Swallow — operator preference is best-effort.
    }
}

export function formatRelativeAge(ageMs: number): string {
    if (ageMs < 5_000) return "just now";
    if (ageMs < 60_000) return `${Math.round(ageMs / 1000)}s ago`;
    if (ageMs < 3_600_000) return `${Math.round(ageMs / 60_000)}m ago`;
    return `${Math.round(ageMs / 3_600_000)}h ago`;
}

// useRefreshInterval — co-located so callers that need polling
// behaviour (refetchInterval on a useQuery) get the same persisted
// state as the MetaStrip selector without re-implementing it.
//
//   const { effectiveMs, chooseInterval } = useRefreshInterval(
//       PLUGIN_ID, agentID, /* default */ 5000,
//   );
//   const query = useQuery({
//       ...,
//       refetchInterval: effectiveMs > 0 ? effectiveMs : false,
//   });
//   <MetaStrip
//       dataUpdatedAt={query.dataUpdatedAt}
//       isFetching={query.isFetching}
//       onRefresh={() => query.refetch()}
//       intervalMs={effectiveMs}
//       onIntervalChange={chooseInterval}
//   />
export function useRefreshInterval(
    pluginID: string,
    agentID: string,
    defaultMs: number,
): { effectiveMs: number; chooseInterval: (ms: number) => void } {
    // null = no operator preference saved → use defaultMs. A
    // persisted 0 means the operator explicitly chose "Off"; that
    // survives even when it happens to match the default.
    const [override, setOverride] = useState<number | null>(() =>
        readPersistedInterval(pluginID, agentID),
    );
    const effectiveMs = override !== null ? override : defaultMs;

    function chooseInterval(ms: number) {
        setOverride(ms);
        writePersistedInterval(pluginID, agentID, ms);
    }

    return { effectiveMs, chooseInterval };
}

export interface MetaStripProps {
    dataUpdatedAt: number;
    isFetching: boolean;
    onRefresh: () => void;
    intervalMs: number;
    onIntervalChange: (ms: number) => void;
}

export function MetaStrip({
    dataUpdatedAt,
    isFetching,
    onRefresh,
    intervalMs,
    onIntervalChange,
}: MetaStripProps) {
    // Tick every 5s so the relative time string ("12s ago", "3m
    // ago", …) doesn't go stale between refetches.
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        const id = setInterval(() => setNow(Date.now()), 5000);
        return () => clearInterval(id);
    }, []);

    const updatedLabel =
        dataUpdatedAt > 0
            ? `Updated ${formatRelativeAge(now - dataUpdatedAt)}`
            : "";

    return (
        <div
            data-testid="rpc-meta-strip"
            style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: space[3],
                fontSize: 12,
                color: palette.textMuted,
            }}
        >
            <span aria-live="polite">{updatedLabel}</span>
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                }}
            >
                <Button
                    variant="ghost"
                    size="icon"
                    onClick={onRefresh}
                    disabled={isFetching}
                    aria-label="Refresh now"
                >
                    <RefreshCw
                        className={
                            isFetching
                                ? "size-4 animate-spin"
                                : "size-4"
                        }
                    />
                </Button>
                <Label
                    htmlFor="rpc-refresh-interval"
                    style={{ fontSize: 11, color: palette.textMuted }}
                >
                    Auto refresh
                </Label>
                <select
                    id="rpc-refresh-interval"
                    aria-label="Auto refresh interval"
                    value={String(intervalMs)}
                    onChange={(e) =>
                        onIntervalChange(Number(e.target.value))
                    }
                    style={{
                        height: 28,
                        padding: "0 6px",
                        background: palette.surface,
                        color: palette.textPrimary,
                        border: `1px solid ${palette.border}`,
                        borderRadius: 6,
                    }}
                >
                    {(() => {
                        const knownValues = new Set(
                            INTERVAL_OPTIONS.map((o) => o.ms),
                        );
                        const orphan = !knownValues.has(intervalMs);
                        const opts = orphan
                            ? [
                                  ...INTERVAL_OPTIONS,
                                  {
                                      ms: intervalMs,
                                      label: `${Math.round(intervalMs / 1000)}s`,
                                  },
                              ]
                            : INTERVAL_OPTIONS;
                        return opts.map((opt) => (
                            <option key={opt.ms} value={String(opt.ms)}>
                                {opt.label}
                            </option>
                        ));
                    })()}
                </select>
            </div>
        </div>
    );
}
