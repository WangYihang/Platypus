import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

vi.mock("../lib/auth", () => ({
    login: vi.fn(),
    bootstrap: vi.fn(),
}));

vi.mock("../lib/servers", () => ({
    getServer: () => null,
    listServers: () => [],
    defaultServerURL: () => "http://127.0.0.1:7331",
}));

import Login from "./Login";

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

// "First-time setup" reads as "I'm setting up Platypus from scratch
// right now", which is wrong — this tab is for bootstrapping the
// FIRST admin against an already-deployed server using the bootstrap
// secret. Renaming the tab to "Bootstrap admin" matches the code's
// own internal vocabulary (bootstrap, BootstrapFormValues) and
// stops new operators interpreting the tab as a server-install flow.

describe("<Login>", () => {
    it("labels the bootstrap tab as 'Bootstrap admin' (not 'First-time setup')", () => {
        renderInRouter(<Login onLoggedIn={vi.fn()} />);
        expect(
            screen.getByRole("tab", { name: /bootstrap admin/i }),
        ).toBeInTheDocument();
        expect(
            screen.queryByRole("tab", { name: /first-time setup/i }),
        ).toBeNull();
    });
});
