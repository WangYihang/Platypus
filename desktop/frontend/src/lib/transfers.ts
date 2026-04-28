// transfers.ts — REST + WS data layer for the file-transfer task list.
//
// Two pieces:
//
//   createTransfersStore({ projectId?, hostId? })
//     A scoped, observable list of transfers. The constructor decides
//     which REST endpoint to call (per-host > per-project > global)
//     based on the supplied filters; load() seeds the list from REST,
//     after which an internal WS subscription keeps it in sync.
//     Snapshot is sorted newest-first by started_at.
//
//   cancelTransfer({ projectId, transferId })
//     POSTs to the cancel endpoint; resolves on 202 Accepted, rejects
//     with the server's body text on any other status.
//
// Designed to be UI-framework agnostic — the React file browser and
// the future transfers tab both subscribe via the same store.

import { authFetch, authJSON } from "./auth";
import { NotifyEvent, onNotify } from "./notify";

export type TransferDirection = "download" | "upload";
export type TransferKind = "file" | "archive" | "folder";
export type TransferStatus = "pending" | "running" | "done" | "failed" | "canceled";

export interface TransferItem {
    id: string;
    project_id: string;
    host_id: string;
    user_id: string;
    direction: TransferDirection;
    kind: TransferKind;
    format: string;
    paths: string[];
    status: TransferStatus;
    bytes_transferred: number;
    total_bytes: number;
    error_message?: string;
    started_at: string;
    finished_at?: string;
}

export interface TransfersStoreOptions {
    projectId?: string;
    hostId?: string;
}

export interface TransfersStore {
    /** Replace the in-memory list with the REST snapshot. Must be
     * called once before subscribers see any data. */
    load(): Promise<void>;
    /** Current rows, newest-first. Returned array is a copy — safe
     * to mutate. */
    snapshot(): TransferItem[];
    /** Subscribe to changes. The callback fires after every WS event
     * that lands in scope. Returns an unsubscribe handle. */
    subscribe(fn: (rows: TransferItem[]) => void): () => void;
    /** Tear down the WS subscription. Idempotent. */
    dispose(): void;
}

/**
 * createTransfersStore wires up a REST seed + WS subscription scoped
 * to the supplied filters. Out-of-scope WS events are ignored so a
 * "this host's transfers" page doesn't churn on activity from other
 * hosts in the same project.
 */
export function createTransfersStore(opts: TransfersStoreOptions): TransfersStore {
    let rows: TransferItem[] = [];
    const subscribers = new Set<(rows: TransferItem[]) => void>();
    let unsubWS: (() => void) | null = null;

    function notifyAll() {
        const snap = rows.slice();
        for (const fn of subscribers) {
            try {
                fn(snap);
            } catch (err) {
                // eslint-disable-next-line no-console
                console.error("transfers subscriber threw:", err);
            }
        }
    }

    function listURL(): string {
        if (opts.hostId && opts.projectId) {
            return `/api/v1/projects/${encodeURIComponent(opts.projectId)}/hosts/${encodeURIComponent(opts.hostId)}/transfers`;
        }
        if (opts.projectId) {
            return `/api/v1/projects/${encodeURIComponent(opts.projectId)}/transfers`;
        }
        return "/api/v1/transfers";
    }

    function inScope(it: TransferItem): boolean {
        if (opts.projectId && it.project_id !== opts.projectId) return false;
        if (opts.hostId && it.host_id !== opts.hostId) return false;
        return true;
    }

    function applyEvent(it: TransferItem) {
        if (!inScope(it)) return;
        const idx = rows.findIndex((r) => r.id === it.id);
        if (idx >= 0) {
            rows[idx] = it;
        } else {
            rows.push(it);
        }
        // Newest-first by started_at; ties broken by id for stability.
        rows.sort((a, b) => {
            if (a.started_at === b.started_at) return a.id < b.id ? 1 : -1;
            return a.started_at < b.started_at ? 1 : -1;
        });
        notifyAll();
    }

    function ensureSubscribed() {
        if (unsubWS) return;
        unsubWS = onNotify(NotifyEvent.FileTransferUpdated, (data: unknown) => {
            const it = data as TransferItem;
            if (!it || typeof it.id !== "string") return;
            applyEvent(it);
        });
    }

    return {
        async load() {
            const resp = await authJSON<{ items?: TransferItem[] }>(listURL());
            rows = (resp.items || []).slice();
            // Server already returns newest-first, but defensively
            // sort so the contract is the same regardless of source.
            rows.sort((a, b) => (a.started_at < b.started_at ? 1 : -1));
            ensureSubscribed();
            notifyAll();
        },
        snapshot() {
            return rows.slice();
        },
        subscribe(fn) {
            subscribers.add(fn);
            ensureSubscribed();
            // Push the current snapshot immediately so subscribers
            // don't miss state-at-subscribe.
            try {
                fn(rows.slice());
            } catch (err) {
                // eslint-disable-next-line no-console
                console.error("transfers initial push:", err);
            }
            return () => subscribers.delete(fn);
        },
        dispose() {
            if (unsubWS) {
                unsubWS();
                unsubWS = null;
            }
            subscribers.clear();
        },
    };
}

export interface CancelTransferOptions {
    projectId: string;
    transferId: string;
}

export async function cancelTransfer(opts: CancelTransferOptions): Promise<void> {
    const resp = await authFetch(
        `/api/v1/projects/${encodeURIComponent(opts.projectId)}/transfers/${encodeURIComponent(opts.transferId)}/cancel`,
        { method: "POST" },
    );
    if (resp.status === 202) return;
    const body = await resp.text().catch(() => "");
    throw new Error(body || `cancel: HTTP ${resp.status}`);
}

// --- Display helpers --------------------------------------------------
//
// Pinned by transfers.test.ts. The header bug from production: archive
// downloads stream gzip-compressed bodies, so `bytes_transferred` (the
// compressed wire count) drifts from `total_bytes` (the uncompressed
// pre-scan total). The status bar / drawer / /transfers page all share
// these helpers so the UI can never accidentally show "180 / 48 ·
// 100% · done" again.

const TERMINAL_STATUSES: ReadonlySet<TransferStatus> = new Set([
    "done",
    "failed",
    "canceled",
]);

/**
 * transferProgressPct is the canonical "what does the bar render?"
 * helper. Returns:
 *   * `null` for indeterminate progress (running with no known total)
 *   * `100` for terminal `done` rows regardless of byte mismatch
 *   * the clamped 0..100 percentage otherwise
 *
 * The clamp matters: backends that overshoot (e.g. compressed stream
 * larger than scan total) used to render 375% bars before this lived
 * in one place.
 */
export function transferProgressPct(it: TransferItem): number | null {
    if (it.status === "done") return 100;
    if (it.total_bytes <= 0) return null;
    const raw = (it.bytes_transferred / it.total_bytes) * 100;
    return Math.max(0, Math.min(100, Math.floor(raw)));
}

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

/**
 * transferDisplaySize formats the size cell. Hides the denominator
 * when total is unknown OR when the numerator overshoots — so the
 * operator sees a monotonically growing number rather than a fake
 * "X / Y" mismatch.
 */
export function transferDisplaySize(it: TransferItem): string {
    const transferred = formatBytes(it.bytes_transferred);
    const knownTotal = it.total_bytes > 0;
    const overshoot = knownTotal && it.bytes_transferred > it.total_bytes;
    if (!knownTotal || overshoot) return transferred;
    return `${transferred} / ${formatBytes(it.total_bytes)}`;
}

/**
 * transferElapsed returns a short wall-clock duration. For terminal
 * rows we use `finished_at - started_at`; for running rows we tick
 * against `now` (injected so tests don't have to fake Date).
 *
 * Format follows the existing host uptime convention:
 *   "Xs" / "Mm Ss" / "Hh Mm" — sub-minute precision drops past 1h.
 */
export function transferElapsed(it: TransferItem, now: Date = new Date()): string {
    const started = Date.parse(it.started_at);
    if (Number.isNaN(started)) return "—";
    let end: number;
    if (TERMINAL_STATUSES.has(it.status) && it.finished_at) {
        end = Date.parse(it.finished_at);
        if (Number.isNaN(end)) end = now.getTime();
    } else {
        end = now.getTime();
    }
    const secs = Math.max(0, Math.floor((end - started) / 1000));
    if (secs < 60) return `${secs}s`;
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins}m ${secs % 60}s`;
    const hours = Math.floor(mins / 60);
    return `${hours}h ${mins % 60}m`;
}
