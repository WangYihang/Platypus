import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

vi.mock("../lib/auth", () => ({
    getSessionUser: () => ({
        id: "u-test",
        username: "ada",
        role: "admin" as const,
    }),
    changePassword: vi.fn().mockResolvedValue(undefined),
}));

const listAccountPATs = vi.fn().mockResolvedValue([]);
const issueAccountPAT = vi.fn().mockResolvedValue({
    token_id: "pat_new",
    token: "pat_new.SECRETSECRETSECRET",
    name: "ci-bot",
    scopes: ["hosts:read"],
    created_at: "2026-04-28T00:00:00Z",
    expires_at: "2026-07-27T00:00:00Z",
});
const revokeAccountPAT = vi.fn().mockResolvedValue(undefined);

vi.mock("../lib/api", () => ({
    listAccountPATs: (...args: unknown[]) => listAccountPATs(...args),
    issueAccountPAT: (...args: unknown[]) => issueAccountPAT(...args),
    revokeAccountPAT: (...args: unknown[]) => revokeAccountPAT(...args),
}));

import Account from "./Account";

// Account is the home of *user-level, server-side* settings. Tabs:
//   1. Identity   — read-only username / role / id card.
//   2. Password   — change-password form (sign-out-everywhere on submit).
//   3. Tokens     — manage user-self PATs (`pat_*`).
//
// The Tokens tab is a real GitHub-style PAT surface: distinct from the
// project-scoped enrollment tokens that admins mint for agent bootstrap.

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("<Account>", () => {
    it("renders an Account title in the page header", () => {
        renderInRouter(<Account />);
        expect(screen.getByText(/^Account$/)).toBeInTheDocument();
    });

    it("renders the tabs container with Identity / Password / Tokens", () => {
        renderInRouter(<Account />);
        expect(screen.getByTestId("account-tabs")).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /identity/i })).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /password/i })).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /tokens/i })).toBeInTheDocument();
    });

    it("Identity tab is the default and shows the username / role card", () => {
        renderInRouter(<Account />);
        expect(
            screen.getByRole("tab", { name: /identity/i, selected: true }),
        ).toBeInTheDocument();
        // Username appears inside the Identity card.
        expect(screen.getByText("ada")).toBeInTheDocument();
    });

    it("switching to Password tab reveals the change-password form", async () => {
        const user = userEvent.setup();
        renderInRouter(<Account />);
        await user.click(screen.getByRole("tab", { name: /password/i }));
        expect(screen.getByLabelText(/current password/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/^new password$/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/confirm new password/i)).toBeInTheDocument();
        expect(
            screen.getByRole("button", { name: /update password/i }),
        ).toBeInTheDocument();
    });

    it("switching to Tokens tab loads the PAT list", async () => {
        const user = userEvent.setup();
        listAccountPATs.mockClear();
        listAccountPATs.mockResolvedValueOnce([]);
        renderInRouter(<Account />);
        await user.click(screen.getByRole("tab", { name: /tokens/i }));
        await waitFor(() => {
            expect(listAccountPATs).toHaveBeenCalled();
        });
        // Empty state copy.
        expect(screen.getByText(/no personal access tokens/i)).toBeInTheDocument();
    });

    it("Tokens tab exposes an Issue button that opens a name + scopes form", async () => {
        const user = userEvent.setup();
        listAccountPATs.mockClear();
        listAccountPATs.mockResolvedValueOnce([]);
        renderInRouter(<Account />);
        await user.click(screen.getByRole("tab", { name: /tokens/i }));
        await user.click(screen.getByRole("button", { name: /issue token/i }));

        // Modal is open and asks for a name.
        expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
        // Scope checkboxes (admin caller has every scope checked by
        // default per plan).
        expect(screen.getByLabelText("hosts:read")).toBeInTheDocument();
        expect(screen.getByLabelText("hosts:exec")).toBeInTheDocument();
    });
});
