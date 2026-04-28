import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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
// list AND a host roster so the Host column can resolve aliases
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

    // G — richer columns: Host alias, Elapsed, Error.
    it("resolves host_id to the host's primary alias", async () => {
        queueResponses(
            [makeRow({ host_id: "h1" })],
            [{ id: "h1", primary_alias: "production-shell" }],
        );
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() =>
            expect(screen.getByTestId("transfer-host-cell").textContent).toMatch(/production-shell/),
        );
    });

    it("renders the error_message in its own column when present", async () => {
        queueResponses([
            makeRow({
                id: "ft-err",
                status: "failed",
                error_message: "permission denied: /root/secret",
            }),
        ]);
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() =>
            expect(screen.getByTestId("transfer-error-cell").textContent).toMatch(
                /permission denied/,
            ),
        );
    });

    it("renders an Elapsed cell that formats the wall-clock duration", async () => {
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
            expect(screen.getByTestId("transfer-elapsed-cell").textContent).toMatch(/1m 30s/),
        );
    });
});
