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
vi.mock("../lib/api", () => ({
    getServerInfo: () => getServerInfoMock(),
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
// count, and clickable version links to the matching GitHub release
// for both server and web. The contract pinned here:
//
//   1. Once a server-info response lands, the bar renders pills for
//      mem / goroutines / uptime / hosts / sessions.
//   2. Server version is a clickable link to /releases/tag/v<ver> on
//      the repo named in git_repo.
//   3. Web version (frontend) is also a clickable link.
//   4. The username span still truncates so a long account name
//      can't push the bar layout around.

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

    it("renders clickable server + web version links pointing at GitHub releases", async () => {
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
        // Both pills are independent — the web version comes from
        // __APP_VERSION__ (set to "test" by vitest.config.ts).
        const web = container.querySelector(
            '[data-testid="status-bar-web-version"]',
        ) as HTMLAnchorElement;
        expect(web.tagName).toBe("A");
        expect(web.getAttribute("href")).toMatch(
            /^https:\/\/github\.com\/WangYihang\/Platypus\/releases\/tag\/v/,
        );
    });

    it("clips overflowing username so it can't push adjacent items out", () => {
        const { container } = renderBar();
        const user = container.querySelector('[data-testid="status-bar-user"]');
        expect(user).not.toBeNull();
        const style = (user as HTMLElement).getAttribute("style") ?? "";
        expect(style).toMatch(/overflow:\s*hidden/i);
        expect(style).toMatch(/text-overflow:\s*ellipsis/i);
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
