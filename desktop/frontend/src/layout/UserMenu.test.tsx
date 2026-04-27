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

// UserMenu is the bottom-of-sidebar identity panel. Its popover used
// to host a "Change password" Dialog inline; that surface moved to a
// dedicated /account page so users go to a real route they can
// bookmark, deep-link, and that distinguishes "this is my server-side
// account" from "this is my browser-local preferences".
//
// Contract pinned here:
//   1. The popover surfaces an Account NavLink → /account
//   2. The popover surfaces a Preferences NavLink → /preferences
//   3. Admin-only items (Manage users, Server settings) only appear
//      for admins.

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

    it("does NOT surface admin-only links for non-admin users", async () => {
        const user = userEvent.setup();
        renderInRouter(<UserMenu user={operatorUser} serverURL="https://example/" />);

        await user.click(screen.getByRole("button", { name: /user menu/i }));

        expect(screen.queryByText(/manage users/i)).toBeNull();
        expect(screen.queryByText(/server settings/i)).toBeNull();
    });

    it("surfaces admin-only links for admin users", async () => {
        const user = userEvent.setup();
        renderInRouter(<UserMenu user={adminUser} serverURL="https://example/" />);

        await user.click(screen.getByRole("button", { name: /user menu/i }));

        expect(screen.getByText(/manage users/i)).toBeInTheDocument();
        expect(screen.getByText(/server settings/i)).toBeInTheDocument();
    });
});
