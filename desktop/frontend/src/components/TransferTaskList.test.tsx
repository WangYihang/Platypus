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
        total_bytes: 1024,
        started_at: "2025-01-01T00:00:00Z",
        ...over,
    };
}

beforeEach(() => {
    authJSONMock.mockReset();
    authFetchMock.mockReset();
});

describe("TransferTaskList", () => {
    it("renders an empty-state when there are no transfers", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [] });
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() =>
            expect(screen.getByText(/no transfers/i)).toBeInTheDocument(),
        );
    });

    it("renders rows from the REST snapshot", async () => {
        authJSONMock.mockResolvedValueOnce({
            items: [
                makeRow({ id: "ft-1", paths: ["/etc"] }),
                makeRow({ id: "ft-2", paths: ["/var/log", "/opt"], status: "done" }),
            ],
        });
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText("/etc")).toBeInTheDocument());
        // Multi-path rows render the count.
        expect(screen.getByText(/\/var\/log/)).toBeInTheDocument();
        // Status pills.
        expect(screen.getByText(/running/i)).toBeInTheDocument();
        expect(screen.getByText(/done/i)).toBeInTheDocument();
    });

    it("shows a progress percentage for running transfers", async () => {
        authJSONMock.mockResolvedValueOnce({
            items: [makeRow({ bytes_transferred: 256, total_bytes: 1024 })],
        });
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText(/25\s*%/)).toBeInTheDocument());
    });

    it("updates a row in place when a WS event arrives", async () => {
        authJSONMock.mockResolvedValueOnce({
            items: [makeRow({ bytes_transferred: 0, total_bytes: 1000 })],
        });
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getByText(/0\s*%/)).toBeInTheDocument());

        (notify as unknown as { __emit: (t: string, d: unknown) => void }).__emit(
            "file_transfer_updated",
            makeRow({ bytes_transferred: 750, total_bytes: 1000 }),
        );
        await waitFor(() => expect(screen.getByText(/75\s*%/)).toBeInTheDocument());
    });

    it("cancels a running transfer when the cancel button is clicked", async () => {
        authJSONMock.mockResolvedValueOnce({ items: [makeRow({ id: "ft-c" })] });
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
        authJSONMock.mockResolvedValueOnce({
            items: [
                makeRow({ id: "ft-d", status: "done" }),
                makeRow({ id: "ft-f", status: "failed" }),
                makeRow({ id: "ft-c", status: "canceled" }),
            ],
        });
        render(<TransferTaskList projectId="p1" />);
        await waitFor(() => expect(screen.getAllByText(/\/etc\/hosts/).length).toBeGreaterThan(0));
        expect(screen.queryByRole("button", { name: /cancel/i })).toBeNull();
    });
});
