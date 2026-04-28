import { describe, expect, it, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";

vi.mock("../lib/api", () => ({
    listHosts: vi.fn().mockResolvedValue([]),
}));

vi.mock("../lib/auth", () => ({
    onActiveChange: () => () => {},
}));

vi.mock("../lib/servers", () => ({
    getActiveServerId: () => null,
}));

vi.mock("@wails/go/app/App", () => ({
    OpenTerminal: vi.fn().mockResolvedValue("term-id"),
    CloseTerminal: vi.fn().mockResolvedValue(undefined),
    ResizeTerminal: vi.fn().mockResolvedValue(undefined),
    SendTerminalInput: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@wails/runtime/runtime", () => ({
    EventsOn: () => () => {},
    EventsOff: () => {},
}));

vi.mock("../layout/ProjectShell", () => ({
    useShell: () => ({
        projects: [],
        project: { id: "p1", slug: "x", name: "X" },
        refresh: vi.fn(),
        loading: false,
    }),
}));

import TerminalDrawer from "./TerminalDrawer";
import { GlobalTerminalProvider, useGlobalTerminal } from "./GlobalTerminalContext";

// TerminalDrawer is now scoped to the host whose detail page the
// operator is currently looking at. The contract pinned here:
//
//   1. Off any host detail page (overview / fleet / activities …),
//      the drawer renders nothing — the bottom of the screen is
//      free for the page itself, even if shells exist on other
//      hosts.
//
//   2. On a host detail page, the drawer renders only that host's
//      shells. Shells on other hosts stay alive in context (ready
//      to be reached via the status-bar terminals popover) but
//      don't bleed into this drawer.
//
//   3. With zero shells anywhere, the drawer renders nothing
//      regardless of route.

interface SeedShell {
    hostId: string;
    projectID: string;
    projectSlug: string;
    sessionHash: string;
    label: string;
}

function Harness({
    initialEntries,
    seedShells,
}: {
    initialEntries: string[];
    seedShells: SeedShell[];
}) {
    return (
        <MemoryRouter initialEntries={initialEntries}>
            <GlobalTerminalProvider>
                <ShellSeeder shells={seedShells} />
                <Routes>
                    <Route path="/projects/:projectSlug/hosts/:hostId/:tab" element={<TerminalDrawer />} />
                    <Route path="*" element={<TerminalDrawer />} />
                </Routes>
            </GlobalTerminalProvider>
        </MemoryRouter>
    );
}

function ShellSeeder({ shells }: { shells: SeedShell[] }) {
    const ctx = useGlobalTerminal();
    if (ctx.shells.length === 0 && shells.length > 0) {
        // Single-shot seeding — useGlobalTerminal's openShell sets
        // drawerOpen=true which is fine for the test fixture.
        for (const s of shells) {
            ctx.openShell(s);
        }
    }
    return null;
}

beforeEach(() => {
    localStorage.clear();
});

describe("<TerminalDrawer> host scoping", () => {
    it("renders nothing when there are no shells anywhere", () => {
        const { container } = render(
            <Harness
                initialEntries={["/projects/x/hosts/h1/info"]}
                seedShells={[]}
            />,
        );
        expect(container.querySelector('[data-testid="terminal-drawer"]')).toBeNull();
    });

    it("renders nothing on a non-host route even if shells exist", () => {
        const { container } = render(
            <Harness
                initialEntries={["/projects/x/overview"]}
                seedShells={[
                    {
                        hostId: "h1",
                        projectID: "p1",
                        projectSlug: "x",
                        sessionHash: "agent-h1",
                        label: "host-1",
                    },
                ]}
            />,
        );
        expect(container.querySelector('[data-testid="terminal-drawer"]')).toBeNull();
    });

    it("renders only the current host's shells on a host detail page", () => {
        const { container } = render(
            <Harness
                initialEntries={["/projects/x/hosts/h1/info"]}
                seedShells={[
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
        const drawer = container.querySelector('[data-testid="terminal-drawer"]');
        expect(drawer).not.toBeNull();
        // Tab labels — only host-1 should be visible.
        expect(drawer!.textContent).toContain("host-1");
        expect(drawer!.textContent).not.toContain("host-2");
    });
});
