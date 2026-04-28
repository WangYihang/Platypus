import { describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
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
    getActiveServer: () => ({ id: "s1", name: "Production", url: "https://example.test" }),
    onServersChange: () => () => {},
}));

vi.mock("../lib/api", () => ({
    getServerInfo: () => Promise.reject(new Error("not under test")),
}));

vi.mock("@wails/runtime/runtime", () => ({
    EventsOn: () => {},
    EventsOff: () => {},
}));

import StatusBar from "./StatusBar";

// On narrow viewports the StatusBar's three flex zones used to fight
// each other for space because no zone capped its width or clipped
// overflowing children. A long public_addr or username pushed
// adjacent items off-screen. The contract pinned here:
//
//   · The user@host group AND the ingress address sit in spans that
//     declare overflow:hidden + textOverflow:ellipsis + nowrap, so
//     the layout degrades gracefully instead of breaking.
//
// This is a regression-only test — it doesn't compute the visible
// width (jsdom has no layout), it asserts the CSS settings the
// browser uses to truncate.

describe("<StatusBar>", () => {
    it("clips overflowing public_addr to keep the right zone tidy", () => {
        const { container } = render(
            <MemoryRouter>
                <StatusBar />
            </MemoryRouter>,
        );
        const ingress = container.querySelector('[data-testid="status-bar-ingress"]');
        expect(ingress).not.toBeNull();
        const style = (ingress as HTMLElement).getAttribute("style") ?? "";
        expect(style).toMatch(/overflow:\s*hidden/i);
        expect(style).toMatch(/text-overflow:\s*ellipsis/i);
        expect(style).toMatch(/white-space:\s*nowrap/i);
    });

    it("clips overflowing username so it can't push adjacent items out", () => {
        const { container } = render(
            <MemoryRouter>
                <StatusBar />
            </MemoryRouter>,
        );
        const user = container.querySelector('[data-testid="status-bar-user"]');
        expect(user).not.toBeNull();
        const style = (user as HTMLElement).getAttribute("style") ?? "";
        expect(style).toMatch(/overflow:\s*hidden/i);
        expect(style).toMatch(/text-overflow:\s*ellipsis/i);
    });
});
