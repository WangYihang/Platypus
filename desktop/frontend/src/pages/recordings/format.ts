import { RecordingStatus } from "../../lib/api";

export const STATUS_TONE: Record<RecordingStatus, "success" | "warning" | "danger" | "neutral"> = {
    completed: "success",
    recording: "warning",
    failed: "danger",
};

// "1m 23s" / "12.4s" / "230ms" — keeps the same visual weight across statuses.
export function formatDuration(ms: number): string {
    if (!Number.isFinite(ms) || ms < 0) return "—";
    if (ms < 1000) return `${ms}ms`;
    const totalSecs = ms / 1000;
    if (totalSecs < 60) return `${totalSecs.toFixed(1)}s`;
    const m = Math.floor(totalSecs / 60);
    const s = Math.floor(totalSecs % 60);
    return s === 0 ? `${m}m` : `${m}m ${s}s`;
}
