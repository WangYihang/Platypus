import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../lib/auth", () => ({
    authJSON: vi.fn(),
    authFetch: vi.fn(),
}));
vi.mock("../lib/notify", () => {
    const listeners = new Map<string, Set<(d: unknown) => void>>();
    return {
        NotifyEvent: { FileTransferUpdated: "file_transfer_updated" },
        onNotify: (type: string, fn: (d: unknown) => void) => {
            const set = listeners.get(type) || new Set();
            set.add(fn);
            listeners.set(type, set);
            return () => set.delete(fn);
        },
        __emit: (type: string, data: unknown) => {
            for (const fn of listeners.get(type) || []) fn(data);
        },
    };
});

import { authJSON, authFetch } from "../lib/auth";
import * as notify from "../lib/notify";
import TransferTaskList from "./TransferTaskList";
import type { TransferItem } from "../lib/transfers";

const authJSONMock = vi.mocked(authJSON);
const authFetchMock = vi.mocked(authFetch);

function makeRow(over: Partial<TransferItem>): TransferItem {
    return {
        id: "ft-1",
        project_id: "p1",
        host_id: "h1",
        user_id: "u1",
        direction: "download",
        kind: "archive",
        format: "tar.gz",
        paths: ["/etc/hosts"],
        status: "running",
        bytes_transferred: 0,
        wire_bytes: 0,
        total_bytes: 1024,
        started_at: "2025-01-01T00:00:00Z",
        ...over,
    };
}

// queueResponses wires up authJSON to return the supplied transfers
// list AND a host roster so the Host detail can resolve aliases
// without each test having to spell that out. Tests pass the rows
// they care about; hosts default to a single seed under id "h1"
// with primary_alias "host-one" so an assertion can pin the alias
// rendering.
function queueResponses(rows: TransferItem[], hosts: Array<{ id: string; primary_alias?: string; hostname?: string }> = [
    { id: "h1", primary_alias: "host-one" },
]) {
    authJSONMock.mockImplementation(async (url: string) => {
        if (url.includes("/transfers")) return { items: rows };
        if (url.includes("/hosts")) return { hosts };
        throw new Error(`unexpected authJSON call: ${url}`);
    });
}

beforeEach(() => {
    authJSONMock.mockReset();
    authFetchMock.mockReset();
});

describe("TransferTaskList", () => {
    it("renders an empty-state when there are no transfers", async () => {
        queueResponses([]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() =>
            expect(screen.getByText(/no transfers/i)).toBeInTheDocument(),
        );
    });

    it("renders rows from the REST snapshot", async () => {
        queueResponses([
            makeRow({ id: "ft-1", paths: ["/etc"] }),
            makeRow({ id: "ft-2", paths: ["/var/log", "/opt"], status: "done" }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText("/etc")).toBeInTheDocument());
        // Multi-path rows render the count.
        expect(screen.getByText(/\/var\/log/)).toBeInTheDocument();
        // Status pills.
        expect(screen.getByText(/running/i)).toBeInTheDocument();
        expect(screen.getByText(/done/i)).toBeInTheDocument();
    });

    it("shows a progress percentage for running transfers", async () => {
        queueResponses([makeRow({ bytes_transferred: 256, total_bytes: 1024 })]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText(/25\s*%/)).toBeInTheDocument());
    });

    it("renders an indeterminate progress bar when total_bytes is zero (compressed archive)", async () => {
        queueResponses([
            makeRow({
                id: "ft-arch",
                bytes_transferred: 180,
                total_bytes: 0,
                status: "running",
            }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => {
            const bar = screen.getByTestId("transfer-progress-bar");
            expect(bar.getAttribute("data-progress")).toBe("indeterminate");
        });
        // The misleading "180 / 48" denominator must NOT appear.
        // (the row's only number is the 180-byte transferred count).
        expect(screen.queryByText(/100\s*%/)).toBeNull();
    });

    it("updates a row in place when a WS event arrives", async () => {
        queueResponses([makeRow({ bytes_transferred: 0, total_bytes: 1000 })]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText(/0\s*%/)).toBeInTheDocument());

        (notify as unknown as { __emit: (t: string, d: unknown) => void }).__emit(
            "file_transfer_updated",
            makeRow({ bytes_transferred: 750, total_bytes: 1000 }),
        );
        await waitFor(() => expect(screen.getByText(/75\s*%/)).toBeInTheDocument());
    });

    it("cancels a running transfer when the cancel button is clicked", async () => {
        queueResponses([makeRow({ id: "ft-c" })]);
        authFetchMock.mockResolvedValueOnce(new Response("", { status: 202 }));

        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText("/etc/hosts")).toBeInTheDocument());

        const btn = screen.getByRole("button", { name: /cancel/i });
        await userEvent.click(btn);

        await waitFor(() =>
            expect(authFetchMock).toHaveBeenCalledWith(
                "/api/v1/projects/p1/transfers/ft-c/cancel",
                expect.objectContaining({ method: "POST" }),
            ),
        );
    });

    it("does NOT show a cancel button on terminal rows", async () => {
        queueResponses([
            makeRow({ id: "ft-d", status: "done" }),
            makeRow({ id: "ft-f", status: "failed" }),
            makeRow({ id: "ft-c", status: "canceled" }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getAllByText(/\/etc\/hosts/).length).toBeGreaterThan(0));
        expect(screen.queryByRole("button", { name: /cancel/i })).toBeNull();
    });

    // ----- Phase 1: slim table + expandable rows -------------------

    it("renders exactly the slim column set (Phase 1)", async () => {
        queueResponses([makeRow({ id: "ft-cols" })]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText("/etc/hosts")).toBeInTheDocument());

        const headers = screen
            .getAllByRole("columnheader")
            .map((h) => h.textContent?.trim() || "");
        // 7 visible columns total: chevron (no label), Path, Progress,
        // Size, Time, Status, Actions. Exactly that count + the chevron
        // — no Direction, Format, Speed, Compression, Started, Error.
        expect(headers.length).toBe(7);

        // Spot-check that the dropped headers are gone.
        expect(
            screen.queryByRole("columnheader", { name: /direction/i }),
        ).toBeNull();
        expect(
            screen.queryByRole("columnheader", { name: /format/i }),
        ).toBeNull();
        expect(
            screen.queryByRole("columnheader", { name: /^speed$/i }),
        ).toBeNull();
        expect(
            screen.queryByRole("columnheader", { name: /compression/i }),
        ).toBeNull();
        expect(
            screen.queryByRole("columnheader", { name: /started/i }),
        ).toBeNull();
        expect(
            screen.queryByRole("columnheader", { name: /^error$/i }),
        ).toBeNull();
    });

    it("tags the leading icon with a direction tone (success/info)", async () => {
        queueResponses([
            makeRow({ id: "ft-dn", direction: "download" }),
            makeRow({ id: "ft-up", direction: "upload" }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() =>
            expect(screen.getAllByText(/\/etc\/hosts/).length).toBe(2),
        );
        const tones = screen
            .getAllByTestId("transfer-direction-icon")
            .map((el) => el.getAttribute("data-direction-tone"));
        expect(tones).toContain("success"); // download → green
        expect(tones).toContain("info"); // upload → blue
    });

    it("renders a relative time cell with the absolute timestamp on title", async () => {
        // started_at 7 s before tickNow — the existing component
        // initialises tickNow to `new Date()`, so we pin the row's
        // started_at to ~7 s earlier than test wall-clock.
        const sevenSecAgo = new Date(Date.now() - 7000).toISOString();
        queueResponses([
            makeRow({ id: "ft-time", started_at: sevenSecAgo, status: "running" }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        const cell = await screen.findByTestId("transfer-time-cell");
        // transferElapsed renders sub-minute durations as "Xs".
        expect(cell.textContent).toMatch(/\b[0-9]{1,2}s\b/);
        // title attribute carries the absolute timestamp so hover
        // recovers the precision the column hides.
        expect(cell.getAttribute("title")).toBeTruthy();
        expect(cell.getAttribute("title")).toContain(sevenSecAgo.slice(0, 10));
    });

    it("stacks size, speed and compression in one cell for compressed archives", async () => {
        // bytes_transferred (source) = 1000, wire_bytes (compressed)
        // = 320 → ratio = 32%. started_at long enough ago that the
        // average speed helper returns a non-null value.
        queueResponses([
            makeRow({
                id: "ft-stk",
                kind: "archive",
                format: "tar.gz",
                bytes_transferred: 1000,
                wire_bytes: 320,
                total_bytes: 4000,
                status: "running",
                started_at: new Date(Date.now() - 5000).toISOString(),
            }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        const cell = await screen.findByTestId("transfer-size-cell");
        // Top line: "X / Y" rendered by transferDisplaySize.
        expect(cell.textContent).toMatch(/\//);
        // Sub-line: bytes/sec.
        expect(cell.textContent).toMatch(/B\/s/);
        // Sub-line: compression ratio token.
        expect(cell.textContent).toMatch(/32\s*%/);
    });

    it("hides the compression token when wire_bytes equals bytes_transferred", async () => {
        queueResponses([
            makeRow({
                id: "ft-plain",
                kind: "file",
                format: "",
                bytes_transferred: 1024,
                wire_bytes: 1024,
                total_bytes: 1024,
                status: "done",
                started_at: new Date(Date.now() - 5000).toISOString(),
                finished_at: new Date(Date.now() - 1000).toISOString(),
            }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        const cell = await screen.findByTestId("transfer-size-cell");
        // Compression-ratio formatting is "<n>%". transferDisplaySize
        // never renders a "%" token, so the absence of "%" in the cell
        // proves the compression line is hidden for plain transfers.
        expect(cell.textContent).not.toMatch(/%/);
    });

    it("expands a row to show host alias + raw bytes + full timestamp on click", async () => {
        const startedAt = "2025-01-01T12:34:56.000Z";
        queueResponses(
            [
                makeRow({
                    id: "ft-expand",
                    host_id: "h1",
                    bytes_transferred: 4096,
                    wire_bytes: 1024,
                    total_bytes: 4096,
                    status: "done",
                    started_at: startedAt,
                    finished_at: "2025-01-01T12:35:30.000Z",
                }),
            ],
            [{ id: "h1", primary_alias: "production-shell" }],
        );
        render(<TransferTaskList projectId="p1" />);
        const row = await screen.findByTestId("transfer-row");

        // Initially: detail row not present.
        expect(screen.queryByTestId("transfer-detail-row")).toBeNull();

        await userEvent.click(row);

        const detail = await screen.findByTestId("transfer-detail-row");
        // Host alias resolved from the host roster.
        expect(detail.textContent).toMatch(/production-shell/);
        // Raw byte counts visible at full precision.
        expect(detail.textContent).toMatch(/4096/);
        expect(detail.textContent).toMatch(/1024/);
        // Full started_at ISO surfaced at full precision.
        expect(detail.textContent).toMatch(/12:34:56/);

        // aria-expanded reflects state.
        expect(row.getAttribute("aria-expanded")).toBe("true");

        // Click again → detail row gone, aria-expanded flips back.
        await userEvent.click(row);
        await waitFor(() =>
            expect(screen.queryByTestId("transfer-detail-row")).toBeNull(),
        );
        expect(row.getAttribute("aria-expanded")).toBe("false");
    });

    it("surfaces the error message inline below the row when present", async () => {
        queueResponses([
            makeRow({
                id: "ft-err",
                status: "failed",
                error_message: "permission denied: /root/secret",
            }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        // Inline error doesn't require expansion — operators need to
        // see what failed at a glance.
        const inline = await screen.findByTestId("transfer-error-inline");
        expect(inline.textContent).toMatch(/permission denied/);
    });

    it("clicking the cancel button does not expand the row", async () => {
        queueResponses([makeRow({ id: "ft-no-toggle", status: "running" })]);
        authFetchMock.mockResolvedValueOnce(new Response("", { status: 202 }));
        render(<TransferTaskList projectId="p1" />);
        const row = await screen.findByTestId("transfer-row");

        const btn = within(row).getByRole("button", { name: /cancel/i });
        await userEvent.click(btn);

        // Cancel posted; expand state did not flip.
        await waitFor(() =>
            expect(authFetchMock).toHaveBeenCalledWith(
                expect.stringMatching(/cancel$/),
                expect.objectContaining({ method: "POST" }),
            ),
        );
        expect(screen.queryByTestId("transfer-detail-row")).toBeNull();
        expect(row.getAttribute("aria-expanded")).toBe("false");
    });

    it("renders an Elapsed-style time cell (running rows tick relative)", async () => {
        // Pin started_at so transferElapsed returns a stable string;
        // for a 90-second-old finished row the helper renders "1m 30s".
        queueResponses([
            makeRow({
                id: "ft-el",
                status: "done",
                started_at: "2025-01-01T00:00:00Z",
                finished_at: "2025-01-01T00:01:30Z",
            }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() =>
            expect(screen.getByTestId("transfer-time-cell").textContent).toMatch(
                /1m 30s/,
            ),
        );
    });
});
