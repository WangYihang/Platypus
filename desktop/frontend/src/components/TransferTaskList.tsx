import { useEffect, useMemo, useState } from "react";

import { palette, radius, space } from "../layout/theme";
import { humanizeError } from "../lib/humanizeError";
import { type Host, listHosts } from "../lib/api";
import {
    cancelTransfer,
    createTransfersStore,
    formatBytesPerSec,
    formatCompressionRatio,
    transferAverageSpeed,
    transferCompressionRatio,
    transferDisplaySize,
    transferElapsed,
    transferProgressPct,
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

function formatPaths(paths: string[]): string {
    if (paths.length === 0) return "";
    if (paths.length === 1) return paths[0];
    return `${paths[0]} (+${paths.length - 1} more)`;
}

/**
 * TransferTaskList renders the file_transfers list as a dense table.
 * Today this only powers the global /transfers nav route — the
 * per-host tab in HostView was removed because it duplicated the
 * always-on TransfersDrawer. Subscribes to /notify so the table
 * re-renders as transfers progress + finish + get cancelled.
 *
 * Columns: Host (alias) · Paths · Direction · Format · Progress ·
 * Size · Speed · Compression · Elapsed · Status · Error · Started ·
 * Actions.
 *
 * Speed is `bytes_transferred / elapsed` (source throughput); the
 * compression cell is `wire_bytes / bytes_transferred` and only
 * shows for archive transfers where the encoder actually shrinks
 * (or, rarely, inflates) the body — plain transfers render an
 * em-dash so the column doesn't add noise.
 */
export default function TransferTaskList({ projectId, hostId }: Props) {
    const store = useMemo(
        () => createTransfersStore({ projectId, hostId }),
        [projectId, hostId],
    );
    const [rows, setRows] = useState<TransferItem[]>(() => store.snapshot());
    const [loaded, setLoaded] = useState(false);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [hostsByID, setHostsByID] = useState<Record<string, Host>>({});

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

    // Resolve host_id → primary alias / hostname so the Host column
    // is human-readable. Cached at mount; the project's host list
    // changes slowly enough that a one-shot fetch is fine. Failures
    // fall back silently to the raw id.
    useEffect(() => {
        if (!projectId) return;
        let cancelled = false;
        listHosts(projectId)
            .then((list) => {
                if (cancelled) return;
                const byID: Record<string, Host> = {};
                for (const h of list) byID[h.id] = h;
                setHostsByID(byID);
            })
            .catch(() => {});
        return () => {
            cancelled = true;
        };
    }, [projectId]);

    // Tick once a second so the Elapsed column for in-flight rows
    // updates live. We re-render the whole table — fine for the
    // dozens-of-rows case the page surfaces.
    const [tickNow, setTickNow] = useState(() => new Date());
    useEffect(() => {
        const id = window.setInterval(() => setTickNow(new Date()), 1000);
        return () => window.clearInterval(id);
    }, []);

    function hostLabel(it: TransferItem): string {
        const h = hostsByID[it.host_id];
        if (!h) return it.host_id ? `${it.host_id.slice(0, 8)}…` : "—";
        return h.primary_alias || h.hostname || `${h.id.slice(0, 8)}…`;
    }

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
                        <th style={thStyle}>Host</th>
                        <th style={thStyle}>Paths</th>
                        <th style={thStyle}>Direction</th>
                        <th style={thStyle}>Format</th>
                        <th style={thStyle}>Progress</th>
                        <th style={thStyle}>Size</th>
                        <th style={thStyle}>Speed</th>
                        <th style={thStyle}>Compression</th>
                        <th style={thStyle}>Elapsed</th>
                        <th style={thStyle}>Status</th>
                        <th style={thStyle}>Error</th>
                        <th style={thStyle}>Started</th>
                        <th style={thStyle}>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {rows.map((it) => (
                        <tr key={it.id} style={trStyle} data-testid="transfer-row">
                            <td style={tdStyle} data-testid="transfer-host-cell" title={it.host_id}>
                                {hostLabel(it)}
                            </td>
                            <td style={tdMonoStyle} title={it.paths.join("\n")}>
                                {formatPaths(it.paths)}
                            </td>
                            <td style={tdStyle}>
                                <span style={{ textTransform: "capitalize" }}>{it.direction}</span>
                            </td>
                            <td style={tdStyle}>{it.format || "—"}</td>
                            <td style={tdStyle}>
                                <ProgressBar pct={transferProgressPct(it)} status={it.status} />
                            </td>
                            <td style={tdStyle}>{transferDisplaySize(it)}</td>
                            <td style={tdStyle} data-testid="transfer-speed-cell">
                                {formatBytesPerSec(transferAverageSpeed(it, tickNow))}
                            </td>
                            <td style={tdStyle} data-testid="transfer-compression-cell">
                                {formatCompressionRatio(transferCompressionRatio(it))}
                            </td>
                            <td style={tdStyle} data-testid="transfer-elapsed-cell">
                                {transferElapsed(it, tickNow)}
                            </td>
                            <td style={tdStyle}>
                                <StatusPill tone={STATUS_TONES[it.status]}>
                                    {it.status}
                                </StatusPill>
                            </td>
                            <td
                                style={tdStyle}
                                data-testid="transfer-error-cell"
                                title={it.error_message || undefined}
                            >
                                {it.error_message ? (
                                    <span style={errorTextStyle}>{it.error_message}</span>
                                ) : (
                                    "—"
                                )}
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
    pct: number | null;
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
    const indeterminate = pct === null;
    return (
        <div
            style={progressTrackStyle}
            data-testid="transfer-progress-bar"
            data-progress={indeterminate ? "indeterminate" : String(pct)}
        >
            {indeterminate ? (
                <div
                    className="transfers-indeterminate"
                    style={{
                        position: "absolute",
                        top: 0,
                        bottom: 0,
                        width: "30%",
                        background: fill,
                        borderRadius: radius.pill,
                    }}
                />
            ) : (
                <div
                    style={{
                        width: `${pct}%`,
                        height: "100%",
                        background: fill,
                        borderRadius: radius.pill,
                        transition: "width 200ms ease-out",
                    }}
                />
            )}
            <span style={progressLabelStyle}>{indeterminate ? "…" : `${pct}%`}</span>
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
