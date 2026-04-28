import { describe, expect, it, vi, beforeEach } from "vitest";
import { act, render, waitFor } from "@testing-library/react";
import {
    MemoryRouter,
    Outlet,
    Route,
    Routes,
    useNavigate,
} from "react-router-dom";
import { useEffect } from "react";

vi.mock("../lib/api", () => ({
    listHosts: vi.fn().mockResolvedValue([]),
}));

vi.mock("../lib/auth", () => ({
    onActiveChange: () => () => {},
}));

vi.mock("../lib/servers", () => ({
    getActiveServerId: () => null,
}));

const OpenTerminal = vi.fn().mockResolvedValue("term-id");
const CloseTerminal = vi.fn().mockResolvedValue(undefined);
const ResizeTerminal = vi.fn().mockResolvedValue(undefined);
const SendTerminalInput = vi.fn().mockResolvedValue(undefined);

vi.mock("@wails/go/app/App", () => ({
    OpenTerminal: (...args: unknown[]) => OpenTerminal(...args),
    CloseTerminal: (...args: unknown[]) => CloseTerminal(...args),
    ResizeTerminal: (...args: unknown[]) => ResizeTerminal(...args),
    SendTerminalInput: (...args: unknown[]) => SendTerminalInput(...args),
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

// TerminalDrawer is host-scoped *visually* — it only reveals on the
// host detail page whose hostId is in the URL — but the underlying
// <Terminal> components must stay mounted across route changes and
// host switches so the xterm/WebSocket (and the operator's tmux
// client on the agent) survive. The contract pinned here:
//
//   1. With zero shells anywhere, the drawer renders nothing.
//
//   2. Off any host detail page (overview / fleet / activities …),
//      the drawer container stays in the DOM — so <Terminal>
//      children stay mounted — but it is visually hidden
//      (data-active="false", height 0, visibility:hidden) so the
//      bottom of the screen is free for the page itself.
//
//   3. On a host detail page, the drawer is active (data-active=
//      "true") and the tab bar lists only that host's shells.
//      Shells on other hosts are still mounted (display:none)
//      so they survive a host switch.

interface SeedShell {
    hostId: string;
    projectID: string;
    projectSlug: string;
    sessionHash: string;
    label: string;
}

// Harness mirrors the real ProjectShell layout: the drawer is mounted
// in a layout Route alongside the page <Outlet />, so route changes
// only swap the Outlet's content while the drawer instance survives.
// That's load-bearing for the persistence guarantee — if the drawer
// itself were re-mounted on every route change, the bug we're fixing
// here would still happen. Wrapping in a layout Route also gives
// useParams() inside <TerminalDrawer> the same matched-route context
// it has in production (the inner :hostId route).
function Harness({
    initialEntries,
    seedShells,
    navigateRef,
}: {
    initialEntries: string[];
    seedShells: SeedShell[];
    navigateRef?: { current: ((to: string) => void) | null };
}) {
    return (
        <MemoryRouter initialEntries={initialEntries}>
            <GlobalTerminalProvider>
                <ShellSeeder shells={seedShells} />
                <Routes>
                    <Route element={<ShellLayout />}>
                        <Route
                            path="/projects/:projectSlug/hosts/:hostId/:tab"
                            element={<NavigateProbe target={navigateRef} label="host" />}
                        />
                        <Route
                            path="*"
                            element={<NavigateProbe target={navigateRef} label="other" />}
                        />
                    </Route>
                </Routes>
            </GlobalTerminalProvider>
        </MemoryRouter>
    );
}

function ShellLayout() {
    return (
        <>
            <Outlet />
            <TerminalDrawer />
        </>
    );
}

function NavigateProbe({
    target,
    label,
}: {
    target?: { current: ((to: string) => void) | null };
    label: string;
}) {
    const navigate = useNavigate();
    useEffect(() => {
        if (target) target.current = (to) => navigate(to);
    }, [navigate, target]);
    return <div data-testid={`page-${label}`}>{label}</div>;
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
    OpenTerminal.mockClear();
    CloseTerminal.mockClear();
    ResizeTerminal.mockClear();
    SendTerminalInput.mockClear();
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

    it("keeps the drawer mounted but visually hidden on a non-host route so terminals survive", () => {
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
        const drawer = container.querySelector(
            '[data-testid="terminal-drawer"]',
        ) as HTMLElement | null;
        // Container stays in the DOM — that's how <Terminal>
        // children survive a route change away from the host page.
        expect(drawer).not.toBeNull();
        // …but it is visually hidden so the bottom of the screen
        // is free for the page itself.
        expect(drawer!.dataset.active).toBe("false");
        expect(drawer!.style.height).toBe("0px");
        expect(drawer!.style.visibility).toBe("hidden");
        // Tab bar is suppressed so off-host shells don't leak into
        // the chrome of an unrelated page.
        expect(drawer!.textContent).not.toContain("host-1");
    });

    // Regression for the bug where navigating off the host page
    // disposed the xterm + WebSocket, which kicked the operator's
    // tmux client on the agent. Pin: navigate host → overview →
    // host MUST NOT close and reopen the underlying terminal.
    it("does not tear down or recreate the terminal when navigating away and back", async () => {
        const navigateRef: { current: ((to: string) => void) | null } = {
            current: null,
        };
        render(
            <Harness
                initialEntries={["/projects/x/hosts/h1/info"]}
                navigateRef={navigateRef}
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

        // Initial mount on the host page opens exactly one terminal.
        await waitFor(() => expect(OpenTerminal).toHaveBeenCalledTimes(1));
        expect(CloseTerminal).not.toHaveBeenCalled();

        // Navigate to a non-host route — the drawer hides but the
        // <Terminal> stays mounted, so no Close + no second Open.
        await act(async () => {
            navigateRef.current!("/projects/x/overview");
        });

        // Navigate back. If the bug were still here, this is when
        // a fresh OpenTerminal call would happen.
        await act(async () => {
            navigateRef.current!("/projects/x/hosts/h1/info");
        });

        expect(OpenTerminal).toHaveBeenCalledTimes(1);
        expect(CloseTerminal).not.toHaveBeenCalled();
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
        const drawer = container.querySelector(
            '[data-testid="terminal-drawer"]',
        ) as HTMLElement | null;
        expect(drawer).not.toBeNull();
        expect(drawer!.dataset.active).toBe("true");
        // Tab labels — only host-1 should be visible in the tab bar.
        expect(drawer!.textContent).toContain("host-1");
        expect(drawer!.textContent).not.toContain("host-2");
    });
});
