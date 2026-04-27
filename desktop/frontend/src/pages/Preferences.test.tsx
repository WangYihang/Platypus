import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import Preferences from "./Preferences";

// Preferences is the home of *client-local* settings (UI density,
// terminal font, default Fleet view, …). The contract this test
// pins:
//   1. The page surfaces a "this browser only" scope hint, so users
//      stop looking for these as project- or account-level settings.
//   2. It exposes the three tabs that previously lived (incorrectly)
//      under /projects/:slug/settings — Display, Terminal, Behaviour.
// If a future refactor moves a tab elsewhere, this test fails loudly.

function renderInRouter(ui: React.ReactElement) {
    return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe("<Preferences>", () => {
    it("renders a 'this browser only' scope hint in the page header", () => {
        renderInRouter(<Preferences />);
        expect(
            screen.getByText(/this browser only/i),
        ).toBeInTheDocument();
    });

    it("exposes Display, Terminal, and Behaviour tabs", () => {
        renderInRouter(<Preferences />);
        expect(screen.getByRole("tab", { name: /display/i })).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /^terminal$/i })).toBeInTheDocument();
        expect(screen.getByRole("tab", { name: /behaviour/i })).toBeInTheDocument();
    });
});
