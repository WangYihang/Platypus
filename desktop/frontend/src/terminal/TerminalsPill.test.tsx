import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

vi.mock("../lib/auth", () => ({
    onActiveChange: () => () => {},
}));

vi.mock("../lib/servers", () => ({
    getActiveServerId: () => null,
}));

import TerminalsPill from "./TerminalsPill";
import {
    GlobalTerminalProvider,
    useGlobalTerminal,
    OpenShellInput,
} from "./GlobalTerminalContext";

// TerminalsPill is the always-visible cross-host index. The drawer
// is host-scoped now, so without this pill operators would lose
// sight of shells on hosts they're not currently looking at. The
// pill lives in the status bar and:
//
//   1. Shows a numeric count of every open shell.
//   2. Renders nothing when no shells are open (so the bar isn't
//      cluttered on a fresh project).
//   3. On click, opens a popover that lists shells grouped by host;
//      each row carries a deterministic colour drawn from the same
//      palette the drawer uses, so colours match across surfaces.

function Seeder({ shells }: { shells: OpenShellInput[] }) {
    const ctx = useGlobalTerminal();
    if (ctx.shells.length === 0 && shells.length > 0) {
        for (const s of shells) ctx.openShell(s);
    }
    return null;
}

function Harness({ shells }: { shells: OpenShellInput[] }) {
    return (
        <MemoryRouter>
            <GlobalTerminalProvider>
                <Seeder shells={shells} />
                <TerminalsPill />
            </GlobalTerminalProvider>
        </MemoryRouter>
    );
}

beforeEach(() => {
    localStorage.clear();
});

describe("<TerminalsPill>", () => {
    it("renders nothing when there are no open shells", () => {
        const { container } = render(<Harness shells={[]} />);
        expect(
            container.querySelector('[data-testid="terminals-pill"]'),
        ).toBeNull();
    });

    it("shows the total count when shells exist", () => {
        render(
            <Harness
                shells={[
                    {
                        hostId: "h1",
                        projectID: "p1",
                        projectSlug: "x",
                        sessionHash: "agent-h1",
                        label: "host-1",
                    },
                    {
                        hostId: "h2",
                        projectID: "p1",
                        projectSlug: "x",
                        sessionHash: "agent-h2",
                        label: "host-2",
                    },
                ]}
            />,
        );
        const pill = screen.getByTestId("terminals-pill");
        expect(pill.textContent).toMatch(/2/);
    });

    it("opens a popover listing shells grouped by host on click", () => {
        render(
            <Harness
                shells={[
                    {
                        hostId: "h1",
                        projectID: "p1",
                        projectSlug: "x",
                        sessionHash: "agent-h1",
                        label: "host-1",
                    },
                    {
                        hostId: "h2",
                        projectID: "p1",
                        projectSlug: "x",
                        sessionHash: "agent-h2",
                        label: "host-2",
                    },
                ]}
            />,
        );
        fireEvent.click(screen.getByTestId("terminals-pill"));
        // Both labels should be reachable via popover content.
        expect(screen.getAllByText(/host-1/).length).toBeGreaterThan(0);
        expect(screen.getAllByText(/host-2/).length).toBeGreaterThan(0);
    });
});
