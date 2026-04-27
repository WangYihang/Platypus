// Small helpers for relative-time rendering. Extracted so the three
// places that used to inline dayjs + relativeTime share one import.

import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(relativeTime);

// A host is "online" if its newest session's last_seen_at falls within
// this window. Matches the 60s cadence the agent keepalive runs at.
export const ONLINE_WINDOW_MS = 60_000;

// fromNow renders an ISO-8601 timestamp (or Date) as "3 minutes ago".
// Returns "—" for missing input so callers can always splice the result
// into a UI cell.
export function fromNow(value: string | Date | null | undefined): string {
    if (!value) return "—";
    return dayjs(value).fromNow();
}

// isOnline decides the green/grey presence dot. Accepts a loosely-typed
// timestamp for ergonomic use directly with response bodies.
export function isOnline(lastSeenAt: string | Date | null | undefined): boolean {
    if (!lastSeenAt) return false;
    const t = new Date(lastSeenAt).getTime();
    return Number.isFinite(t) && Date.now() - t < ONLINE_WINDOW_MS;
}

// formatSeconds renders a non-negative integer second count as a human
// duration: "5m", "1h 30m", "2d 3h", "1w 2d", "—" for invalid / blank.
// Built for inline "= …" hints next to TTL inputs (F10) so users can
// type 86400 and see "= 1d" without doing the arithmetic. Caps at the
// two most-significant units to keep the hint terse.
export function formatSeconds(secs: number | null | undefined): string {
    if (secs === null || secs === undefined) return "—";
    if (!Number.isFinite(secs)) return "—";
    const n = Math.floor(secs);
    if (n < 0) return "—";
    if (n === 0) return "0s";

    const units: Array<[label: string, size: number]> = [
        ["w", 7 * 24 * 60 * 60],
        ["d", 24 * 60 * 60],
        ["h", 60 * 60],
        ["m", 60],
        ["s", 1],
    ];
    const parts: string[] = [];
    let rem = n;
    for (const [label, size] of units) {
        if (parts.length >= 2) break;
        const v = Math.floor(rem / size);
        if (v > 0) {
            parts.push(`${v}${label}`);
            rem -= v * size;
        } else if (parts.length > 0) {
            // Once we've emitted a larger unit, skip zeros until we
            // find the next non-zero or run out.
            continue;
        }
    }
    return parts.join(" ") || "0s";
}
