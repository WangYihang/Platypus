import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";

// Stub the transfers store so we can drive the rows + status flowing
// into TransfersPill without going through the WS layer. The pill
// reads `activeCount` from the provider, which is derived from
// `rows.filter(r => r.status==="running"||"pending")`.
const fakeStore = {
    rows: [] as Array<{ id: string; status: string }>,
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
        // Re-export helpers; the pill renders the count only.
        transferProgressPct: () => null,
        transferDisplaySize: () => "",
    };
});

vi.mock("../lib/auth", () => ({
    getSession: () => ({ sessionToken: "tok" }),
    onSessionChange: () => () => {},
}));

import TransfersPill, { TransfersDrawerProvider } from "./TransfersPill";

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
