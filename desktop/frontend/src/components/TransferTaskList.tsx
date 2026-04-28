import { Fragment, useEffect, useMemo, useState } from "react";
import { ArrowDownToLine, ArrowUpFromLine, ChevronRight } from "lucide-react";

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

// Number of visible columns in <thead>. Detail / inline-error rows
// span all of them via colSpan so the layout stays aligned.
const VISIBLE_COLUMNS = 7;

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
 * Columns: chevron · Path (with direction icon) · Progress · Size
 * (stacked: size / speed / compression) · Time (relative duration,
 * absolute timestamp on title=) · Status · Actions.
 *
 * Click a row to expand a detail panel with host alias, raw byte
 * counts (source + wire), full ISO timestamps, format, full path
 * list, and the error message in full. The data is the same shape
 * the drawer surfaces; the table is the audit-log view, the drawer
 * is the live "what's happening *now*" view.
 *
 * Operators see a row's error_message on a dedicated red sub-line
 * underneath the row without having to expand — failed transfers
 * shouldn't need an extra click to diagnose.
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
    // Set of transfer ids currently expanded into the detail row.
    const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

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

    // Resolve host_id → primary alias / hostname so the expanded
    // row + the path tooltip stay human-readable. Cached at mount;
    // the project's host list changes slowly enough that a one-shot
    // fetch is fine. Failures fall back silently to the raw id.
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

    // Tick once a second so the Time cell for in-flight rows updates
    // live. We re-render the whole table — fine for the dozens-of-rows
    // case the page surfaces.
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

    function toggleExpanded(id: string) {
        setExpanded((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            return next;
        });
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
                        <th style={thChevronStyle} aria-label="" />
                        <th style={thStyle}>Path</th>
                        <th style={thStyle}>Progress</th>
                        <th style={thNumStyle}>Size</th>
                        <th style={thNumStyle}>Time</th>
                        <th style={thStyle}>Status</th>
                        <th style={thStyle}>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {rows.map((it) => (
                        <Fragment key={it.id}>
                            <Row
                                it={it}
                                expanded={expanded.has(it.id)}
                                tickNow={tickNow}
                                onToggle={() => toggleExpanded(it.id)}
                                onCancel={() => onCancel(it)}
                            />
                            {it.error_message ? (
                                <tr
                                    style={inlineErrorRowStyle}
                                    data-testid="transfer-error-inline"
                                >
                                    <td colSpan={VISIBLE_COLUMNS} style={inlineErrorCellStyle}>
                                        {it.error_message}
                                    </td>
                                </tr>
                            ) : null}
                            {expanded.has(it.id) ? (
                                <DetailRow it={it} hostLabel={hostLabel(it)} />
                            ) : null}
                        </Fragment>
                    ))}
                </tbody>
            </table>
        </div>
    );
}

interface RowProps {
    it: TransferItem;
    expanded: boolean;
    tickNow: Date;
    onToggle: () => void;
    onCancel: () => void;
}

function Row({ it, expanded, tickNow, onToggle, onCancel }: RowProps) {
    const directionTone = it.direction === "upload" ? "info" : "success";
    const directionColor =
        it.direction === "upload" ? palette.info : palette.success;
    const DirectionIcon =
        it.direction === "upload" ? ArrowUpFromLine : ArrowDownToLine;
    const isCompressed =
        it.wire_bytes > 0 && it.wire_bytes !== it.bytes_transferred;
    const speedText = formatBytesPerSec(transferAverageSpeed(it, tickNow));
    const ratioText = formatCompressionRatio(transferCompressionRatio(it));
    const subParts: string[] = [];
    if (speedText !== "—") subParts.push(speedText);
    if (isCompressed && ratioText !== "—") subParts.push(ratioText);
    const startedAbs = new Date(it.started_at).toISOString();

    return (
        <tr
            style={expanded ? trExpandedStyle : trStyle}
            data-testid="transfer-row"
            aria-expanded={expanded}
            onClick={onToggle}
        >
            <td style={tdChevronStyle} aria-hidden>
                <ChevronRight
                    className="size-3.5"
                    style={{
                        transform: expanded ? "rotate(90deg)" : "rotate(0deg)",
                        transition: "transform 120ms ease-out",
                        color: palette.textMuted,
                    }}
                />
            </td>
            <td style={tdPathStyle} title={it.paths.join("\n")}>
                <span style={pathInnerStyle}>
                    <DirectionIcon
                        className="size-3.5"
                        data-testid="transfer-direction-icon"
                        data-direction-tone={directionTone}
                        style={{ color: directionColor, flexShrink: 0 }}
                    />
                    <span style={pathTextStyle}>{formatPaths(it.paths)}</span>
                </span>
            </td>
            <td style={tdStyle}>
                <ProgressBar pct={transferProgressPct(it)} status={it.status} />
            </td>
            <td
                style={tdNumStyle}
                data-testid="transfer-size-cell"
            >
                <div style={sizeMainStyle}>{transferDisplaySize(it)}</div>
                {subParts.length > 0 ? (
                    <div style={sizeSubStyle}>{subParts.join(" · ")}</div>
                ) : null}
            </td>
            <td
                style={tdNumStyle}
                data-testid="transfer-time-cell"
                title={startedAbs}
            >
                {transferElapsed(it, tickNow)}
            </td>
            <td style={tdStyle}>
                <StatusPill tone={STATUS_TONES[it.status]}>{it.status}</StatusPill>
            </td>
            <td style={tdStyle}>
                {!TERMINAL.has(it.status) ? (
                    <Button
                        size="sm"
                        variant="outline"
                        onClick={(e) => {
                            // Stop the row's onClick from also toggling the
                            // detail row — Cancel and expand are separate
                            // affordances on the same surface.
                            e.stopPropagation();
                            onCancel();
                        }}
                    >
                        Cancel
                    </Button>
                ) : null}
            </td>
        </tr>
    );
}

function DetailRow({
    it,
    hostLabel,
}: {
    it: TransferItem;
    hostLabel: string;
}) {
    const finishedAbs = it.finished_at
        ? new Date(it.finished_at).toISOString()
        : "—";
    return (
        <tr style={detailRowStyle} data-testid="transfer-detail-row">
            <td colSpan={VISIBLE_COLUMNS} style={detailCellStyle}>
                <dl style={detailGridStyle}>
                    <DetailKV k="Host" v={`${hostLabel} · ${it.host_id}`} />
                    <DetailKV k="Format" v={it.format || "—"} />
                    <DetailKV k="Direction" v={it.direction} />
                    <DetailKV k="Kind" v={it.kind} />
                    <DetailKV k="Started" v={new Date(it.started_at).toISOString()} />
                    <DetailKV k="Finished" v={finishedAbs} />
                    <DetailKV
                        k="Source bytes"
                        v={String(it.bytes_transferred)}
                    />
                    <DetailKV k="Wire bytes" v={String(it.wire_bytes)} />
                    <DetailKV
                        k="Total bytes"
                        v={it.total_bytes > 0 ? String(it.total_bytes) : "unknown"}
                    />
                    {it.paths.length > 1 ? (
                        <DetailKV
                            k="Paths"
                            v={it.paths.join("\n")}
                            wide
                        />
                    ) : null}
                    {it.error_message ? (
                        <DetailKV
                            k="Error"
                            v={it.error_message}
                            wide
                            tone="danger"
                        />
                    ) : null}
                </dl>
            </td>
        </tr>
    );
}

function DetailKV({
    k,
    v,
    wide,
    tone,
}: {
    k: string;
    v: string;
    wide?: boolean;
    tone?: "danger";
}) {
    return (
        <div
            style={{
                gridColumn: wide ? "span 2" : "span 1",
                display: "flex",
                gap: space[2],
            }}
        >
            <dt style={detailKeyStyle}>{k}</dt>
            <dd
                style={{
                    ...detailValueStyle,
                    color: tone === "danger" ? palette.danger : palette.textPrimary,
                    whiteSpace: wide ? "pre-wrap" : "nowrap",
                    overflow: wide ? "visible" : "hidden",
                    textOverflow: wide ? "clip" : "ellipsis",
                    margin: 0,
                }}
            >
                {v}
            </dd>
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
            style={progressWrapperStyle}
            data-testid="transfer-progress-bar"
            data-progress={indeterminate ? "indeterminate" : String(pct)}
        >
            <div style={progressTrackStyle}>
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
            </div>
            <span style={progressLabelStyle}>{indeterminate ? "…" : `${pct}%`}</span>
        </div>
    );
}

const tableStyle: React.CSSProperties = {
    width: "100%",
    borderCollapse: "collapse",
    fontSize: 13,
};

// Padding values must carry their `px` unit because the template
// literal turns `space[2]` (the number 8) into the unitless string
// "8 12" — invalid CSS, silently dropped by the browser, leaving
// the table headers running together with no horizontal breathing
// room. Use the same explicit-unit pattern the rest of the
// codebase uses (Card.tsx, EmptyState.tsx, …).
const thStyle: React.CSSProperties = {
    textAlign: "left",
    padding: `${space[2]}px ${space[3]}px`,
    fontSize: 11,
    fontWeight: 600,
    color: palette.textMuted,
    textTransform: "uppercase",
    letterSpacing: 0.4,
    borderBottom: `1px solid ${palette.border}`,
    whiteSpace: "nowrap",
};

const thNumStyle: React.CSSProperties = {
    ...thStyle,
    textAlign: "right",
};

// Chevron column is a fixed-width gutter — empty header label so the
// affordance is purely the chevron itself.
const thChevronStyle: React.CSSProperties = {
    ...thStyle,
    width: 24,
    padding: `${space[2]}px ${space[1]}px`,
};

const trStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
    cursor: "pointer",
};

const trExpandedStyle: React.CSSProperties = {
    ...trStyle,
    background: palette.surfaceHover,
};

const tdStyle: React.CSSProperties = {
    padding: `${space[2]}px ${space[3]}px`,
    color: palette.textPrimary,
    verticalAlign: "middle",
};

const tdChevronStyle: React.CSSProperties = {
    ...tdStyle,
    padding: `${space[2]}px ${space[1]}px`,
    width: 24,
    color: palette.textMuted,
};

// Numeric cells: right-aligned so the operator can scan a column of
// sizes/speeds/percentages without their eye jumping. nowrap stops
// "38.7 MB / 38.7 MB" from breaking across two lines when the
// viewport gets tight. tabular-nums keeps digits the same width so
// adjacent rows line up vertically.
const tdNumStyle: React.CSSProperties = {
    ...tdStyle,
    textAlign: "right",
    whiteSpace: "nowrap",
    fontVariantNumeric: "tabular-nums",
};

const tdPathStyle: React.CSSProperties = {
    ...tdStyle,
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    color: palette.textSecondary,
    maxWidth: 360,
    overflow: "hidden",
};

const pathInnerStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    gap: space[2],
    minWidth: 0,
};

const pathTextStyle: React.CSSProperties = {
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
};

// Stacked size cell: the canonical "X / Y" display on top, smaller
// muted "B/s · ratio" sub-line beneath. Both lines right-aligned via
// the parent <td>'s textAlign.
const sizeMainStyle: React.CSSProperties = {
    fontVariantNumeric: "tabular-nums",
};

const sizeSubStyle: React.CSSProperties = {
    fontSize: 11,
    color: palette.textMuted,
    fontVariantNumeric: "tabular-nums",
    marginTop: 1,
};

// Inline error sub-row is a single full-width red strip immediately
// under the row so the operator sees what failed without expanding.
const inlineErrorRowStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
};

const inlineErrorCellStyle: React.CSSProperties = {
    padding: `${space[1]}px ${space[3]}px ${space[2]}px ${space[3]}px`,
    color: palette.danger,
    fontSize: 12,
    background: palette.surface,
};

// Expanded detail row: a single full-width cell containing a
// two-column key-value grid. Same border treatment as the rest of
// the table so the visual rhythm doesn't break.
const detailRowStyle: React.CSSProperties = {
    borderBottom: `1px solid ${palette.border}`,
    background: palette.surfaceHover,
};

const detailCellStyle: React.CSSProperties = {
    padding: `${space[3]}px ${space[5]}px`,
};

const detailGridStyle: React.CSSProperties = {
    display: "grid",
    gridTemplateColumns: "1fr 1fr",
    gap: `${space[2]}px ${space[5]}px`,
    margin: 0,
    fontSize: 12,
};

const detailKeyStyle: React.CSSProperties = {
    color: palette.textMuted,
    minWidth: 100,
    whiteSpace: "nowrap",
    fontWeight: 500,
};

const detailValueStyle: React.CSSProperties = {
    color: palette.textPrimary,
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    fontVariantNumeric: "tabular-nums",
};

// The track + label sit side-by-side inside the cell so the label
// can't overflow into the next column the way `position: absolute;
// right: -34px` did. Fixed track width + min-width on the wrapper
// stops the column from collapsing when the table fights for space.
const progressWrapperStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    gap: space[2],
    minWidth: 180,
};

const progressTrackStyle: React.CSSProperties = {
    position: "relative",
    width: 140,
    height: 6,
    background: palette.border,
    borderRadius: radius.pill,
    overflow: "hidden",
    flexShrink: 0,
};

const progressLabelStyle: React.CSSProperties = {
    fontSize: 11,
    color: palette.textMuted,
    fontVariantNumeric: "tabular-nums",
    minWidth: 32,
    textAlign: "right",
};
