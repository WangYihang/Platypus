import { useEffect, useMemo, useState } from "react";

import { palette, radius, space } from "../layout/theme";
import { humanizeError } from "../lib/humanizeError";
import {
    cancelTransfer,
    createTransfersStore,
    type TransferItem,
    type TransferStatus,
} from "../lib/transfers";
import { Button } from "./ui/button";
import EmptyState from "./EmptyState";
import StatusPill from "./StatusPill";

interface Props {
    /** Restrict the list to a single project. Omit for the global view. */
    projectId?: string;
    /** Restrict further to a single host. */
    hostId?: string;
}

// Status-pill tone for each transfer status. canceled gets the
// "warning" tone so it visually separates from a hard failure.
const STATUS_TONES: Record<
    TransferStatus,
    "neutral" | "success" | "warning" | "danger" | "info"
> = {
    pending: "neutral",
    running: "info",
    done: "success",
    failed: "danger",
    canceled: "warning",
};

const TERMINAL: ReadonlySet<TransferStatus> = new Set([
    "done",
    "failed",
    "canceled",
]);

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

function formatPaths(paths: string[]): string {
    if (paths.length === 0) return "";
    if (paths.length === 1) return paths[0];
    return `${paths[0]} (+${paths.length - 1} more)`;
}

function progressPct(it: TransferItem): number {
    if (it.total_bytes <= 0) return 0;
    return Math.min(100, Math.round((it.bytes_transferred / it.total_bytes) * 100));
}

/**
 * TransferTaskList renders the file_transfers list as a dense table.
 * Used on the per-host transfers tab AND the global /transfers route;
 * the only difference is which scope the embedded store is created
 * with. Subscribes to /notify so the table re-renders as transfers
 * progress + finish + get cancelled.
 */
export default function TransferTaskList({ projectId, hostId }: Props) {
    const store = useMemo(
        () => createTransfersStore({ projectId, hostId }),
        [projectId, hostId],
    );
    const [rows, setRows] = useState<TransferItem[]>(() => store.snapshot());
    const [loaded, setLoaded] = useState(false);
    const [loadError, setLoadError] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        store.load().then(
            () => {
                if (!cancelled) setLoaded(true);
            },
            (err) => {
                if (!cancelled) setLoadError(humanizeError(err));
            },
        );
        const unsub = store.subscribe(setRows);
        return () => {
            cancelled = true;
            unsub();
            store.dispose();
        };
    }, [store]);

    async function onCancel(it: TransferItem) {
        try {
            await cancelTransfer({
                projectId: it.project_id,
                transferId: it.id,
            });
        } catch (err) {
            // Surface as console for now; toast wiring lives at the
            // page layer. Failures here are usually "already finished".
            // eslint-disable-next-line no-console
            console.error("cancel transfer:", err);
        }
    }

    if (loadError) {
        return (
            <EmptyState
                title="Could not load transfers"
                description={loadError}
            />
        );
    }
    if (loaded && rows.length === 0) {
        return (
            <EmptyState
                title="No transfers yet"
                description="Downloads and uploads from the file browser appear here."
            />
        );
    }

    return (
        <div style={{ overflowX: "auto" }}>
            <table style={tableStyle}>
                <thead>
                    <tr>
                        <th style={thStyle}>Paths</th>
                        <th style={thStyle}>Direction</th>
                        <th style={thStyle}>Format</th>
                        <th style={thStyle}>Progress</th>
                        <th style={thStyle}>Size</th>
                        <th style={thStyle}>Status</th>
                        <th style={thStyle}>Started</th>
                        <th style={thStyle}>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {rows.map((it) => (
                        <tr key={it.id} style={trStyle}>
                            <td style={tdMonoStyle} title={it.paths.join("\n")}>
                                {formatPaths(it.paths)}
                            </td>
                            <td style={tdStyle}>
                                <span style={{ textTransform: "capitalize" }}>{it.direction}</span>
                            </td>
                            <td style={tdStyle}>{it.format || "—"}</td>
                            <td style={tdStyle}>
                                <ProgressBar pct={progressPct(it)} status={it.status} />
                            </td>
                            <td style={tdStyle}>
                                {formatBytes(it.bytes_transferred)}
                                {it.total_bytes > 0 && it.total_bytes !== it.bytes_transferred ? (
                                    <span style={{ color: palette.textMuted }}>
                                        {" "}/ {formatBytes(it.total_bytes)}
                                    </span>
                                ) : null}
                            </td>
                            <td style={tdStyle}>
                                <StatusPill tone={STATUS_TONES[it.status]}>
                                    {it.status}
                                </StatusPill>
                                {it.error_message ? (
                                    <span style={errorTextStyle} title={it.error_message}>
                                        {" "}{it.error_message}
                                    </span>
                                ) : null}
                            </td>
                            <td style={tdStyle}>
                                {new Date(it.started_at).toLocaleString()}
                            </td>
                            <td style={tdStyle}>
                                {!TERMINAL.has(it.status) ? (
                                    <Button
                                        size="sm"
                                        variant="outline"
                                        onClick={() => onCancel(it)}
                                    >
                                        Cancel
                                    </Button>
                                ) : null}
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

interface ProgressBarProps {
    pct: number;
    status: TransferStatus;
}

function ProgressBar({ pct, status }: ProgressBarProps) {
    const fill =
        status === "failed"
            ? palette.danger
            : status === "canceled"
                ? palette.warning
                : status === "done"
                    ? palette.success
                    : palette.info;
    return (
        <div style={progressTrackStyle}>
            <div
                style={{
                    width: `${pct}%`,
                    height: "100%",
                    background: fill,
                    borderRadius: radius.pill,
                    transition: "width 200ms ease-out",
                }}
            />
            <span style={progressLabelStyle}>{pct}%</span>
        </div>
    );
}

const tableStyle: React.CSSProperties = {
    width: "100%",
    borderCollapse: "collapse",
    fontSize: 13,
};

const thStyle: React.CSSProperties = {
    textAlign: "left",
    padding: `${space[2]} ${space[3]}`,
    fontSize: 11,
    fontWeight: 600,
    color: palette.textMuted,
    textTransform: "uppercase",
    letterSpacing: 0.4,
    borderBottom: `1px solid ${palette.border}`,
};

const trStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
};

const tdStyle: React.CSSProperties = {
    padding: `${space[2]} ${space[3]}`,
    color: palette.textPrimary,
    verticalAlign: "middle",
};

const tdMonoStyle: React.CSSProperties = {
    ...tdStyle,
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    color: palette.textSecondary,
    maxWidth: 320,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
};

const errorTextStyle: React.CSSProperties = {
    color: palette.danger,
    fontSize: 11,
    marginLeft: space[1],
};

const progressTrackStyle: React.CSSProperties = {
    position: "relative",
    width: 140,
    height: 6,
    background: palette.border,
    borderRadius: radius.pill,
    overflow: "hidden",
};

const progressLabelStyle: React.CSSProperties = {
    position: "absolute",
    right: -34,
    top: -7,
    fontSize: 11,
    color: palette.textMuted,
    minWidth: 30,
};
