import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

vi.mock("../lib/auth", () => ({
    getSessionUser: () => ({
        id: "u-test",
        username: "ada",
        role: "admin" as const,
    }),
    changePassword: vi.fn().mockResolvedValue(undefined),
}));

import Account from "./Account";

// Account is the home of *user-level, server-side* settings — at the
// moment that's just "Change password", lifted out of the UserMenu
// popover Dialog. The page needs:
//   1. A scope hint that says "Account" (so users distinguish this
//      from /preferences, which is browser-local).
//   2. A change-password form with the three expected fields:
//      current password, new password, confirm.

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("<Account>", () => {
    it("renders an Account title in the page header", () => {
        renderInRouter(<Account />);
        // PageHeader renders the title as a styled <div>, not a real
        // <h1>. Assert via the visible text rather than the heading
        // role until the primitive grows a semantic heading.
        expect(screen.getByText(/^Account$/)).toBeInTheDocument();
    });

    it("renders the change-password form with all three fields", () => {
        renderInRouter(<Account />);
        expect(screen.getByLabelText(/current password/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/^new password$/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/confirm new password/i)).toBeInTheDocument();
        expect(
            screen.getByRole("button", { name: /update password/i }),
        ).toBeInTheDocument();
    });
});
