import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

vi.mock("../lib/auth", () => ({
    getSession: () => ({
        serverURL: "https://example.test",
        sessionToken: "tok",
        user: { id: "u1", username: "ada", role: "admin" as const },
    }),
    onSessionChange: () => () => {},
    onActiveChange: () => () => {},
}));

vi.mock("../lib/servers", () => ({
    getActiveServer: () => ({
        id: "s1",
        name: "Production",
        url: "https://example.test",
    }),
    onServersChange: () => () => {},
}));

const fakeInfo = {
    version: "0.4.2",
    commit: "abc1234",
    date: "2026-01-01T00:00:00Z",
    git_repo: "WangYihang/Platypus",
    started_at: "2026-04-28T00:00:00Z",
    started_at_unix: Math.floor(Date.UTC(2026, 3, 28, 0, 0, 0) / 1000),
    public_addr: "203.0.113.5:13337",
    session_count: 3,
    live_session_count: 3,
    total_session_count: 47,
    host_count: 12,
    live_host_count: 9,
    goroutines: 124,
    mem_alloc_bytes: 47 * 1024 * 1024,
};

const getServerInfoMock = vi.fn();
const transfersStore = {
    snapshot: vi.fn(() => []),
    load: vi.fn(() => Promise.resolve()),
    subscribe: vi.fn(() => () => {}),
    dispose: vi.fn(),
};
vi.mock("../lib/api", () => ({
    getServerInfo: () => getServerInfoMock(),
}));
vi.mock("../lib/transfers", () => ({
    createTransfersStore: () => transfersStore,
    cancelTransfer: vi.fn(),
}));

vi.mock("@wails/runtime/runtime", () => ({
    EventsOn: () => {},
    EventsOff: () => {},
}));

import StatusBar from "./StatusBar";

beforeEach(() => {
    getServerInfoMock.mockReset();
    getServerInfoMock.mockResolvedValue(fakeInfo);
});

// The status bar surfaces the server's runtime telemetry the operator
// reads at-a-glance: memory, goroutines, uptime, host count, session
// count, and clickable version links to the matching GitHub release.
// Layout contracts pinned here:
//
//   1. Once a server-info response lands, the bar renders pills for
//      mem / goroutines / uptime / hosts / sessions.
//   2. mem and grtn pills include a small inline sparkline of the
//      last samples so operators see the trend at a glance.
//   3. Server version is a clickable link to /releases/tag/v<ver> on
//      the repo named in git_repo. The standalone "web vX.Y" pill is
//      gone — it always rendered as v0.0.0 (vite reads package.json
//      which dev never bumps), so it was pure visual noise.
//   4. The current-user chip lives inside the status-dot popover, not
//      inline — so a long username can't push the bar layout around.

function renderBar() {
    return render(
        <MemoryRouter>
            <StatusBar />
        </MemoryRouter>,
    );
}

describe("<StatusBar> telemetry pills", () => {
    it("renders memory / goroutines / uptime / hosts / sessions once info loads", async () => {
        const { container } = renderBar();
        await waitFor(() => {
            expect(
                container.querySelector('[data-testid="status-bar-mem"]'),
            ).not.toBeNull();
        });
        const mem = container.querySelector('[data-testid="status-bar-mem"]')!;
        expect(mem.textContent).toMatch(/MiB|KiB|GiB|B/);
        const grtn = container.querySelector('[data-testid="status-bar-goroutines"]')!;
        expect(grtn.textContent).toMatch(/124/);
        const up = container.querySelector('[data-testid="status-bar-uptime"]')!;
        expect(up).not.toBeNull();
        const hosts = container.querySelector('[data-testid="status-bar-hosts"]')!;
        // Live / total in the form "9 / 12".
        expect(hosts.textContent).toMatch(/9.*\/.*12/);
        const sess = container.querySelector('[data-testid="status-bar-sessions"]')!;
        // Live / total in the form "3 / 47".
        expect(sess.textContent).toMatch(/3.*\/.*47/);
    });

    it("draws inline sparklines inside the mem and grtn pills", async () => {
        const { container } = renderBar();
        await waitFor(() => {
            expect(
                container.querySelector('[data-testid="status-bar-mem"] [data-testid="sparkline"]'),
            ).not.toBeNull();
        });
        expect(
            container.querySelector('[data-testid="status-bar-goroutines"] [data-testid="sparkline"]'),
        ).not.toBeNull();
    });

    it("renders both server (linked) and web (commit) version pills", async () => {
        const { container } = renderBar();
        await waitFor(() => {
            expect(
                container.querySelector('[data-testid="status-bar-server-version"]'),
            ).not.toBeNull();
        });
        const server = container.querySelector(
            '[data-testid="status-bar-server-version"]',
        ) as HTMLAnchorElement;
        expect(server.tagName).toBe("A");
        expect(server.getAttribute("href")).toBe(
            "https://github.com/WangYihang/Platypus/releases/tag/v0.4.2",
        );
        // Web pill is restored — uses __APP_COMMIT__ (vite-injected,
        // "test" in vitest.config.ts) since __APP_VERSION__ is always
        // 0.0.0 in dev. It's plain text, not a link (no commit page).
        const web = container.querySelector(
            '[data-testid="status-bar-web-version"]',
        );
        expect(web).not.toBeNull();
        expect(web!.tagName).not.toBe("A");
        expect(web!.textContent).toMatch(/web/);
        expect(web!.textContent).toMatch(/test/); // __APP_COMMIT__ in tests
    });

    it("does not render the username inline — it lives in the status-dot popover", async () => {
        const { container } = renderBar();
        await waitFor(() => {
            expect(
                container.querySelector('[data-testid="status-bar-mem"]'),
            ).not.toBeNull();
        });
        // The inline pill is gone; the only place the username may
        // still appear is inside the popover (rendered into a portal
        // when the dot is clicked, so it isn't in the static DOM).
        expect(
            container.querySelector('[data-testid="status-bar-user"]'),
        ).toBeNull();
    });

    it("does not render an inline ingress pill — it lives in the status-dot popover now", async () => {
        const { container } = renderBar();
        await waitFor(() => {
            expect(
                container.querySelector('[data-testid="status-bar-mem"]'),
            ).not.toBeNull();
        });
        // The bar may still mention public_addr inside the popover
        // (rendered into a portal only when open), but in the static
        // DOM there is no top-level ingress pill jostling for space.
        expect(
            container.querySelector('[data-testid="status-bar-ingress"]'),
        ).toBeNull();
    });
});
