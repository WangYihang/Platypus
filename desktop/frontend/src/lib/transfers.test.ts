import { describe, expect, it, vi, beforeEach } from "vitest";

// transfers.ts is the data layer for the transfers UI: a tiny store
// that fetches the initial list via REST and then keeps it in sync
// via WebSocket FileTransferUpdated events. The store itself is
// independent of any rendering layer (no React) so we can unit-test
// it without DOM.

vi.mock("./auth", () => ({
    authJSON: vi.fn(),
    authFetch: vi.fn(),
}));

vi.mock("./notify", () => {
    const listeners = new Map<string, Set<(data: unknown) => void>>();
    return {
        NotifyEvent: { FileTransferUpdated: "file_transfer_updated" },
        onNotify: vi.fn((type: string, fn: (data: unknown) => void) => {
            const set = listeners.get(type) || new Set();
            set.add(fn);
            listeners.set(type, set);
            return () => set.delete(fn);
        }),
        // Test-only: emit a fake event.
        __emit: (type: string, data: unknown) => {
            for (const fn of listeners.get(type) || []) fn(data);
        },
    };
});

import { authJSON, authFetch } from "./auth";
import * as notify from "./notify";
import {
    createTransfersStore,
    type TransferItem,
    cancelTransfer,
} from "./transfers";

const authJSONMock = vi.mocked(authJSON);
const authFetchMock = vi.mocked(authFetch);

const baseRow: TransferItem = {
    id: "ft-1",
    project_id: "p1",
    host_id: "h1",
    user_id: "u1",
    direction: "download",
    kind: "archive",
    format: "tar.gz",
    paths: ["/etc"],
    status: "running",
    bytes_transferred: 0,
    total_bytes: 1024,
    started_at: "2025-01-01T00:00:00Z",
};

beforeEach(() => {
    authJSONMock.mockReset();
    authFetchMock.mockReset();
});

describe("transfers store", () => {
    it("loads the initial list from REST scoped to a project", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [baseRow] });
        const store = createTransfersStore({ projectId: "p1" });
        await store.load();
        expect(authJSONMock).toHaveBeenCalledWith(
            "/api/v1/projects/p1/transfers",
        );
        expect(store.snapshot()).toEqual([baseRow]);
    });

    it("loads the per-host list when hostId is supplied", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [baseRow] });
        const store = createTransfersStore({ projectId: "p1", hostId: "h1" });
        await store.load();
        expect(authJSONMock).toHaveBeenCalledWith(
            "/api/v1/projects/p1/hosts/h1/transfers",
        );
    });

    it("hits the global endpoint when neither projectId nor hostId is set", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [baseRow] });
        const store = createTransfersStore({});
        await store.load();
        expect(authJSONMock).toHaveBeenCalledWith("/api/v1/transfers");
    });

    it("merges incoming WS events into the store and notifies subscribers", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [baseRow] });
        const store = createTransfersStore({ projectId: "p1" });
        await store.load();

        const seen: TransferItem[][] = [];
        const unsub = store.subscribe((rows) => seen.push(rows));

        // First event: progress update for ft-1.
        const updated: TransferItem = {
            ...baseRow,
            bytes_transferred: 512,
            status: "running",
        };
        (notify as unknown as { __emit: (t: string, d: unknown) => void }).__emit(
            "file_transfer_updated",
            updated,
        );
        // Second event: a brand-new transfer ft-2 — should appear at the top.
        const newer: TransferItem = { ...baseRow, id: "ft-2", started_at: "2025-01-01T00:00:01Z" };
        (notify as unknown as { __emit: (t: string, d: unknown) => void }).__emit(
            "file_transfer_updated",
            newer,
        );

        unsub();
        const last = seen[seen.length - 1];
        expect(last.find((r) => r.id === "ft-1")?.bytes_transferred).toBe(512);
        expect(last.find((r) => r.id === "ft-2")).toBeDefined();
        // ft-2 is newer (later started_at) so it should sort first.
        expect(last[0].id).toBe("ft-2");
    });

    it("filters WS events by projectId/hostId scope", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [] });
        const store = createTransfersStore({ projectId: "p1", hostId: "h1" });
        await store.load();
        const calls: TransferItem[][] = [];
        store.subscribe((rows) => calls.push(rows));
        // Out-of-scope event: different host.
        (notify as unknown as { __emit: (t: string, d: unknown) => void }).__emit(
            "file_transfer_updated",
            { ...baseRow, id: "ft-x", host_id: "h-other" },
        );
        // In-scope event.
        (notify as unknown as { __emit: (t: string, d: unknown) => void }).__emit(
            "file_transfer_updated",
            { ...baseRow, id: "ft-y", host_id: "h1" },
        );
        const last = calls[calls.length - 1] || [];
        expect(last.find((r) => r.id === "ft-x")).toBeUndefined();
        expect(last.find((r) => r.id === "ft-y")).toBeDefined();
    });
});

describe("cancelTransfer", () => {
    it("POSTs to the project-scoped cancel endpoint", async () => {
        authFetchMock.mockResolvedValueOnce(new Response("", { status: 202 }));
        await cancelTransfer({ projectId: "p1", transferId: "ft-1" });
        expect(authFetchMock).toHaveBeenCalledWith(
            "/api/v1/projects/p1/transfers/ft-1/cancel",
            expect.objectContaining({ method: "POST" }),
        );
    });

    it("rejects when the server returns 404", async () => {
        authFetchMock.mockResolvedValueOnce(
            new Response("transfer not found", { status: 404 }),
        );
        await expect(
            cancelTransfer({ projectId: "p1", transferId: "ft-x" }),
        ).rejects.toThrow(/transfer not found/);
    });
});
