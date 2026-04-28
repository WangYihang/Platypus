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
    formatBytesPerSec,
    formatCompressionRatio,
    transferAverageSpeed,
    transferCompressionRatio,
    transferDisplaySize,
    transferElapsed,
    transferProgressPct,
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
    wire_bytes: 0,
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

// transferProgressPct is the single source of truth for the bar's
// fill ratio. Three contracts pinned here:
//   1. null while running with no known total → indeterminate UI.
//   2. 100 on terminal `done`, regardless of byte mismatch.
//   3. clamped to [0,100] for partial transfers so a compressed
//      stream that overshoots the scan total never lies as 375%.
describe("transferProgressPct", () => {
    it("returns null while running with no known total", () => {
        expect(
            transferProgressPct({ ...baseRow, status: "running", bytes_transferred: 200, total_bytes: 0 }),
        ).toBeNull();
    });

    it("returns 100 when status is done even if total is unknown", () => {
        expect(
            transferProgressPct({ ...baseRow, status: "done", bytes_transferred: 200, total_bytes: 0 }),
        ).toBe(100);
    });

    it("returns 100 when bytes overshoot total (source counter race)", () => {
        // bytes_transferred is now uncompressed source bytes (sampled
        // asynchronously by the agent), so the overshoot path is rare —
        // a file growing during the walk can briefly push us past the
        // pre-scan total. The clamp keeps the bar at 100% on done.
        expect(
            transferProgressPct({
                ...baseRow,
                status: "done",
                total_bytes: 48,
                bytes_transferred: 180,
            }),
        ).toBe(100);
    });

    it("returns the floor of the running ratio for partial transfers", () => {
        expect(
            transferProgressPct({
                ...baseRow,
                status: "running",
                total_bytes: 200,
                bytes_transferred: 50,
            }),
        ).toBe(25);
    });
});

// transferDisplaySize hides the denominator when the wire numbers
// can't be trusted (total unknown OR transferred overshoots) so the
// operator never sees the "180 / 48" mismatch.
describe("transferDisplaySize", () => {
    it("hides the denominator when total_bytes is zero", () => {
        const out = transferDisplaySize({
            ...baseRow,
            bytes_transferred: 200,
            total_bytes: 0,
        });
        expect(out).not.toMatch(/\//);
    });

    it("hides the denominator when transferred overshoots total", () => {
        const out = transferDisplaySize({
            ...baseRow,
            bytes_transferred: 180,
            total_bytes: 48,
            status: "done",
        });
        expect(out).not.toMatch(/\//);
        expect(out).toMatch(/180/);
    });

    it("shows X / Y for a known-size single-file transfer", () => {
        const out = transferDisplaySize({
            ...baseRow,
            kind: "file",
            bytes_transferred: 1024,
            total_bytes: 4096,
            status: "running",
        });
        expect(out).toMatch(/\//);
    });
});

// transferElapsed is `now`-injectable so tests don't fake Date.
describe("transferElapsed", () => {
    const now = new Date("2026-01-01T00:00:30Z");

    it("uses now when the transfer is still running", () => {
        const item = { ...baseRow, started_at: "2026-01-01T00:00:00Z", status: "running" as const };
        expect(transferElapsed(item, now)).toBe("30s");
    });

    it("uses finished_at when the transfer is terminal", () => {
        const item = {
            ...baseRow,
            started_at: "2026-01-01T00:00:00Z",
            finished_at: "2026-01-01T00:02:05Z",
            status: "done" as const,
        };
        expect(transferElapsed(item, now)).toBe("2m 5s");
    });

    it("formats hour-scale durations with h/m", () => {
        const item = {
            ...baseRow,
            started_at: "2026-01-01T00:00:00Z",
            finished_at: "2026-01-01T03:14:00Z",
            status: "done" as const,
        };
        expect(transferElapsed(item, now)).toBe("3h 14m");
    });
});

// transferCompressionRatio surfaces wire/source so the operator can
// see how effective the encoding was. Returns null when the two
// counters are equal (no compression happened) so the UI can hide
// the column for plain transfers.
describe("transferCompressionRatio", () => {
    it("returns null when source bytes are zero", () => {
        expect(
            transferCompressionRatio({ ...baseRow, bytes_transferred: 0, wire_bytes: 0 }),
        ).toBeNull();
    });

    it("returns null when wire equals source (plain transfer)", () => {
        expect(
            transferCompressionRatio({
                ...baseRow,
                kind: "file",
                bytes_transferred: 4096,
                wire_bytes: 4096,
            }),
        ).toBeNull();
    });

    it("returns wire/source when they diverge", () => {
        expect(
            transferCompressionRatio({
                ...baseRow,
                bytes_transferred: 1000,
                wire_bytes: 320,
            }),
        ).toBeCloseTo(0.32);
    });

    it("formatCompressionRatio renders a percentage; null becomes em-dash", () => {
        expect(formatCompressionRatio(0.32)).toBe("32%");
        expect(formatCompressionRatio(null)).toBe("—");
        // tar of many tiny files can overshoot, surface that too.
        expect(formatCompressionRatio(1.05)).toBe("105%");
    });
});

// transferAverageSpeed uses source bytes / wall-clock so it reflects
// "how fast my data is being processed" — the number a user feels.
describe("transferAverageSpeed", () => {
    const start = "2026-01-01T00:00:00Z";
    const now = new Date("2026-01-01T00:00:10Z"); // +10 s

    it("returns null when no source bytes have been seen", () => {
        expect(
            transferAverageSpeed({ ...baseRow, started_at: start, bytes_transferred: 0 }, now),
        ).toBeNull();
    });

    it("returns null when elapsed wall-time is too short to be stable", () => {
        const justStarted = new Date("2026-01-01T00:00:00.100Z");
        expect(
            transferAverageSpeed(
                { ...baseRow, started_at: start, bytes_transferred: 1024 },
                justStarted,
            ),
        ).toBeNull();
    });

    it("computes bytes / elapsed seconds for running rows", () => {
        // 1 MiB processed over 10 s → ~104857.6 B/s.
        const got = transferAverageSpeed(
            { ...baseRow, started_at: start, bytes_transferred: 1024 * 1024 },
            now,
        );
        expect(got).toBeCloseTo(104857.6, 0);
    });

    it("uses finished_at instead of now once the row is terminal", () => {
        const item: TransferItem = {
            ...baseRow,
            started_at: start,
            finished_at: "2026-01-01T00:00:05Z", // +5 s, half the window
            status: "done",
            bytes_transferred: 1024 * 1024,
        };
        const got = transferAverageSpeed(item, now);
        // Same source bytes, half the time → twice the throughput.
        expect(got).toBeCloseTo(209715.2, 0);
    });

    it("formatBytesPerSec emits a short suffix; null becomes em-dash", () => {
        expect(formatBytesPerSec(1024 * 1024)).toMatch(/MB\/s/);
        expect(formatBytesPerSec(null)).toBe("—");
        expect(formatBytesPerSec(0)).toBe("—");
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
