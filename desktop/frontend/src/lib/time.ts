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
