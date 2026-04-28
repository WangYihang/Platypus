import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Stub the transfers store so we can drive the rows + status flowing
// into TransfersPill without going through the WS layer. The pill
// reads `activeCount` from the provider, which is derived from
// `rows.filter(r => r.status==="running"||"pending")`.
type FakeRow = {
    id: string;
    direction?: "download" | "upload";
    status?: string;
    paths?: string[];
    bytes_transferred?: number;
    wire_bytes?: number;
    total_bytes?: number;
    started_at?: string;
    finished_at?: string;
    error_message?: string;
    project_id?: string;
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
};

vi.mock("../lib/transfers", async () => {
    return {
        createTransfersStore: () => fakeStore,
        cancelTransfer: vi.fn(),
        // Re-export helpers; pill + drawer rows render via these.
        transferProgressPct: () => null,
        transferDisplaySize: () => "",
        transferAverageSpeed: () => null,
        transferCompressionRatio: () => null,
        formatBytesPerSec: () => "—",
        formatCompressionRatio: () => "—",
        transferDirectionTone: (it: { direction: string }) =>
            it.direction === "upload" ? "info" : "success",
    };
});

vi.mock("../lib/auth", () => ({
    getSession: () => ({ sessionToken: "tok" }),
    onSessionChange: () => () => {},
}));

import TransfersPill, {
    TransfersDrawer,
    TransfersDrawerProvider,
} from "./TransfersPill";

beforeEach(() => {
    fakeStore.rows = [];
    fakeStore.subscribers.clear();
});

function renderPill() {
    return render(
        <TransfersDrawerProvider>
            <TransfersPill />
        </TransfersDrawerProvider>,
    );
}

function renderPillWithDrawer() {
    return render(
        <TransfersDrawerProvider>
            <TransfersPill />
            <TransfersDrawer />
        </TransfersDrawerProvider>,
    );
}

// Active-state contract:
//   * `data-active="true"` when at least one transfer is running or pending.
//   * `data-active="false"` otherwise.
// The CSS that matches operator-feedback ("太不显眼") rides the same
// attribute so a UI tweak doesn't have to touch this assertion.
describe("<TransfersPill> visibility", () => {
    it("marks the pill inactive when no transfers are running", async () => {
        const { container } = renderPill();
        await waitFor(() => {
            const pill = container.querySelector('[data-testid="transfers-pill"]');
            expect(pill).not.toBeNull();
            expect(pill!.getAttribute("data-active")).toBe("false");
        });
    });

    it("marks the pill active when at least one transfer is running", async () => {
        fakeStore.rows = [{ id: "ft-1", status: "running" }];
        const { container } = renderPill();
        await waitFor(() => {
            const pill = container.querySelector('[data-testid="transfers-pill"]');
            expect(pill!.getAttribute("data-active")).toBe("true");
        });
    });
});

// Direction-tone contract: the drawer's leading direction icon must
// be tagged with data-direction-tone="success" for downloads and
// "info" for uploads, so operators can tell the two apart at a
// glance. We pin the data attribute (not inline-style colour) so a
// theme-token or palette tweak doesn't break the test.
describe("<TransfersDrawer> direction tone", () => {
    it("tags downloads with the success tone and uploads with the info tone", async () => {
        fakeStore.rows = [
            {
                id: "ft-dl",
                direction: "download",
                status: "running",
                paths: ["/etc/hosts"],
                bytes_transferred: 0,
                wire_bytes: 0,
                total_bytes: 0,
                started_at: "2025-01-01T00:00:00Z",
                project_id: "p1",
            },
            {
                id: "ft-up",
                direction: "upload",
                status: "running",
                paths: ["/tmp/note.txt"],
                bytes_transferred: 0,
                wire_bytes: 0,
                total_bytes: 0,
                started_at: "2025-01-01T00:00:01Z",
                project_id: "p1",
            },
        ];

        renderPillWithDrawer();
        // Open the drawer by clicking the pill.
        const pill = await screen.findByTestId("transfers-pill");
        await userEvent.click(pill);

        await waitFor(() =>
            expect(screen.getAllByTestId("transfers-drawer-row").length).toBe(2),
        );

        const tones = screen
            .getAllByTestId("transfers-direction-icon")
            .map((el) => el.getAttribute("data-direction-tone"));
        expect(tones).toContain("success"); // download
        expect(tones).toContain("info"); // upload
    });
});
