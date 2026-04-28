import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Stubbed transfers store: like other status-bar pill tests, we
// drive the rows via a faux store so we don't touch the WS layer.
type FakeRow = {
    id: string;
    status: string;
    direction?: "download" | "upload";
    paths?: string[];
    bytes_transferred?: number;
    wire_bytes?: number;
    total_bytes?: number;
    started_at?: string;
};

const fakeStore = {
    rows: [] as FakeRow[],
    snapshot: vi.fn(() => fakeStore.rows),
    load: vi.fn(() => Promise.resolve()),
    subscribers: new Set<(rows: unknown) => void>(),
    subscribe(fn: (rows: unknown) => void) {
        fakeStore.subscribers.add(fn);
        fn(fakeStore.rows);
        return () => fakeStore.subscribers.delete(fn);
    },
    dispose: vi.fn(),
    push(rows: FakeRow[]) {
        fakeStore.rows = rows;
        for (const fn of fakeStore.subscribers) fn(rows);
    },
};

vi.mock("../lib/transfers", async () => {
    const actual = await vi.importActual<typeof import("../lib/transfers")>(
        "../lib/transfers",
    );
    return {
        ...actual,
        createTransfersStore: () => fakeStore,
    };
});

vi.mock("../lib/auth", () => ({
    getSession: () => ({ sessionToken: "tok" }),
    onSessionChange: () => () => {},
    onActiveChange: () => () => {},
}));

import { TransfersDrawerProvider } from "./TransfersPill";
import TransferThroughputPill from "./TransferThroughputPill";

beforeEach(() => {
    fakeStore.rows = [];
    fakeStore.subscribers.clear();
    // Mock Date.now() only — leave timers real so React's
    // useEffect / state-update cycle keeps flowing and Radix's
    // pointer-event handlers can flush. The pill reads Date.now()
    // directly to stamp samples, so this is sufficient to drive
    // the rate calculation.
    vi.spyOn(Date, "now").mockReturnValue(0);
});

afterEach(() => {
    vi.restoreAllMocks();
});

function setNow(ms: number) {
    (Date.now as unknown as { mockReturnValue: (n: number) => void }).mockReturnValue(
        ms,
    );
}

function renderPill() {
    return render(
        <TransfersDrawerProvider>
            <TransferThroughputPill />
        </TransfersDrawerProvider>,
    );
}

// Phase 4 contract: the pill renders an em-dash when nothing is
// running, ticks up to a real B/s once two samples land in the
// 5-second window, and exposes a Popover with one row per running
// transfer.
describe("<TransferThroughputPill>", () => {
    it("renders idle when there are no running transfers", async () => {
        renderPill();
        const pill = screen.getByTestId("transfer-throughput-pill");
        expect(pill.getAttribute("data-active")).toBe("false");
        expect(pill.textContent).toMatch(/—/);
    });

    it("ticks bytes/sec from two snapshots one second apart", async () => {
        renderPill();
        // Snapshot 1 at t=0 with 0 source bytes — establishes the
        // baseline sample.
        act(() => {
            setNow(0);
            fakeStore.push([
                {
                    id: "ft-1",
                    status: "running",
                    direction: "download",
                    paths: ["/etc/hosts"],
                    bytes_transferred: 0,
                    wire_bytes: 0,
                    total_bytes: 1048576,
                    started_at: "1970-01-01T00:00:00.000Z",
                },
            ]);
        });
        // Advance the wall clock by 1 s; emit snapshot 2 with 1 MiB
        // transferred.
        act(() => {
            setNow(1000);
            fakeStore.push([
                {
                    id: "ft-1",
                    status: "running",
                    direction: "download",
                    paths: ["/etc/hosts"],
                    bytes_transferred: 1048576,
                    wire_bytes: 1048576,
                    total_bytes: 1048576,
                    started_at: "1970-01-01T00:00:00.000Z",
                },
            ]);
        });

        const pill = screen.getByTestId("transfer-throughput-pill");
        await waitFor(() => expect(pill.textContent).toMatch(/MB\/s/));
        // 1 MiB in 1 s ≈ 1.0 MB/s after the formatter rounds.
        expect(pill.textContent).toMatch(/1\.0\s*MB\/s/);
    });

    it("hover popover lists one row per running transfer", async () => {
        renderPill();
        act(() => {
            fakeStore.push([
                {
                    id: "ft-1",
                    status: "running",
                    direction: "download",
                    paths: ["/etc/hosts"],
                    bytes_transferred: 100,
                    wire_bytes: 100,
                    total_bytes: 1000,
                    started_at: "1970-01-01T00:00:00.000Z",
                },
                {
                    id: "ft-2",
                    status: "running",
                    direction: "upload",
                    paths: ["/tmp/note.txt"],
                    bytes_transferred: 50,
                    wire_bytes: 50,
                    total_bytes: 200,
                    started_at: "1970-01-01T00:00:00.000Z",
                },
            ]);
        });
        const trigger = screen.getByTestId("transfer-throughput-pill");
        await userEvent.click(trigger);
        const rows = await screen.findAllByTestId("throughput-row");
        expect(rows.length).toBe(2);
    });
});
