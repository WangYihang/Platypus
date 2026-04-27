import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import RoleHelpIcon from "./RoleHelpIcon";

// RoleHelpIcon is a tiny help-affordance for any column header that
// shows a role label. The contract:
//   1. It exposes an aria-label that names what it explains, so
//      screen readers don't read "icon" or stay silent.
//   2. The accessible name AND the title attribute carry the role
//      summary so non-mouse users still get the explanation.
// The visual presentation (HelpCircle glyph, hover tooltip via Radix)
// is intentionally not pinned — these are presentation details that
// can drift as the design system evolves.

describe("<RoleHelpIcon>", () => {
    it("exposes a Role help affordance", () => {
        render(<RoleHelpIcon />);
        const icon = screen.getByRole("button", { name: /role/i });
        expect(icon).toBeInTheDocument();
    });

    it("includes role names and descriptions in the title attribute", () => {
        render(<RoleHelpIcon />);
        const icon = screen.getByRole("button", { name: /role/i });
        const title = icon.getAttribute("title") ?? "";
        expect(title).toMatch(/admin/i);
        expect(title).toMatch(/operator/i);
        expect(title).toMatch(/viewer/i);
    });
});
