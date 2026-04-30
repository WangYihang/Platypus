import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

vi.mock("../lib/auth", () => ({
    logout: vi.fn().mockResolvedValue(undefined),
    changePassword: vi.fn(),
    SessionUser: undefined,
}));

import UserMenu from "./UserMenu";

const adminUser = {
    id: "u1",
    username: "ada",
    role: "admin" as const,
};

const operatorUser = {
    id: "u2",
    username: "bob",
    role: "operator" as const,
};

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

// UserMenu is the avatar popover in TopBar's right cluster. Personal-
// settings only — Account, Preferences, Logout. Admin destinations
// (Users / Access Control / Settings) used to sit here for admins
// but moved to a dedicated /admin top-tab in the 2026-04 IA pass, so
// the menu is identical for every role.
//
// Contract pinned here:
//   1. The popover surfaces an Account NavLink → /account
//   2. The popover surfaces a Preferences NavLink → /preferences
//   3. Admin-only links no longer appear (regardless of role) —
//      they're a top-level nav-row tab now.

describe("<UserMenu>", () => {
    it("opens the popover and surfaces Account + Preferences links", async () => {
        const user = userEvent.setup();
        renderInRouter(<UserMenu user={operatorUser} serverURL="https://example/" />);

        await user.click(screen.getByRole("button", { name: /user menu/i }));

        const account = screen.getByRole("link", { name: /^account$/i });
        expect(account).toBeInTheDocument();
        expect(account).toHaveAttribute("href", "/account");

        const prefs = screen.getByRole("link", { name: /preferences/i });
        expect(prefs).toBeInTheDocument();
        expect(prefs).toHaveAttribute("href", "/preferences");
    });

    it("does NOT surface admin links for non-admin users", async () => {
        const user = userEvent.setup();
        renderInRouter(<UserMenu user={operatorUser} serverURL="https://example/" />);

        await user.click(screen.getByRole("button", { name: /user menu/i }));

        expect(screen.queryByText(/manage users/i)).toBeNull();
        expect(screen.queryByText(/server settings/i)).toBeNull();
        expect(screen.queryByText(/access control/i)).toBeNull();
    });

    it("does NOT surface admin links for admin users either", async () => {
        // Post 2026-04: Admin lives as a top-tab, not in UserMenu.
        const user = userEvent.setup();
        renderInRouter(<UserMenu user={adminUser} serverURL="https://example/" />);

        await user.click(screen.getByRole("button", { name: /user menu/i }));

        expect(screen.queryByText(/manage users/i)).toBeNull();
        expect(screen.queryByText(/server settings/i)).toBeNull();
        expect(screen.queryByText(/access control/i)).toBeNull();
    });
});
