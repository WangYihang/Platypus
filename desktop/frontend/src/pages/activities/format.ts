import { ActivityOutcome } from "../../lib/api";

// Curated superset of categories the backend writes. Static so the filter
// dropdown renders before the first fetch; the underlying `q` text search
// is the escape hatch for anything not in this list.
export const CATEGORY_OPTIONS = [
    "auth",
    "session",
    "command",
    "file",
    "tunnel",
    "listener",
    "agent",
    "admin",
    "project",
    "server",
    "user",
];

export const OUTCOME_TONE: Record<ActivityOutcome, "success" | "warning" | "danger"> = {
    success: "success",
    denied: "warning",
    error: "danger",
};

export type TimeRange = "24h" | "7d" | "30d" | "all";

// "all" returns null so the server's "no from filter" branch fires.
export function rangeToFrom(range: TimeRange): Date | null {
    const now = Date.now();
    switch (range) {
        case "24h":
            return new Date(now - 24 * 60 * 60 * 1000);
        case "7d":
            return new Date(now - 7 * 24 * 60 * 60 * 1000);
        case "30d":
            return new Date(now - 30 * 24 * 60 * 60 * 1000);
        case "all":
            return null;
    }
}

// <1s → ms; 1–60s → seconds (one decimal); >60s → "Nm Ns".
export function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms} ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
    const mins = Math.floor(ms / 60_000);
    const secs = Math.floor((ms % 60_000) / 1000);
    return `${mins}m ${secs}s`;
}
