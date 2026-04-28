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
